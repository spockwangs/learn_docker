package main

import (
	"fmt"
	"github.com/urfave/cli"
	"log"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"path"
	"net"
)

type RunOptions struct {
	createTty     bool
	containerName string
	containerId   string
	imageName     string
	command       string
	volumes []string
}

type Container struct {
	id string
	pid int
	ip net.IP
	peerName string
}

var runCommand = cli.Command{
	Name:  "run",
	Usage: "Run a container from an image",
	UsageText: `mydocker run [OPTIONS] IMAGE COMMAND`,
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "i",
			Usage: "enable tty",
		},
		cli.StringFlag{
			Name:  "name",
			Usage: "container name",
		},
		cli.StringFlag{
			Name:  "m",
			Usage: "memory limit",
		},
		cli.StringFlag{
			Name:  "cpu-shares",
			Usage: "cpushare",
		},
		cli.IntFlag{
			Name:  "cpu-period",
			Usage: "CPU period in microseconds",
		},
		cli.IntFlag{
			Name:  "cpu-quota",
			Usage: "CPU quota in microseconds",
		},
		cli.StringFlag{
			Name:  "cpuset-cpus",
			Usage: "specify which CPUs to run",
		},
		cli.Float64Flag{
			Name:  "cpus",
			Usage: "specify how many CPUs to run",
		},
		cli.StringFlag{
			Name: "v",
			Usage: "mount volumes",
		},
		cli.StringFlag{
			Name: "net",
			Usage: "specify which network to connect to; `host` means connecting to host network",
		},
	},
	Action: func(ctx *cli.Context) error {
		if len(ctx.Args()) < 2 {
			return fmt.Errorf("Missing image or command")
		}

		runOpts := RunOptions{}
		var argArray []string
		for _, arg := range ctx.Args() {
			argArray = append(argArray, arg)
		}
		runOpts.imageName = argArray[0]
		runOpts.command = strings.Join(argArray[1:], " ")
		runOpts.containerName = ctx.String("name")
		runOpts.createTty = ctx.Bool("i")
		runOpts.containerId = makeContainerId()
		if runOpts.containerName == "" {
			runOpts.containerName = runOpts.containerId
		}
		if ctx.String("v") != "" {
			runOpts.volumes = strings.Split(ctx.String("v"), ":")
			if len(runOpts.volumes) != 2 {
				return fmt.Errorf("bad volumes")
			}
		}

		subsystemConfig := SubsystemConfig{
			cpuShare:  ctx.Int("cpu-shares"),
			memory:    ctx.String("m"),
			cpuPeriod: ctx.Int("cpu-period"),
			cpuQuota:  ctx.Int("cpu-quota"),
			cpus:      ctx.Float64("cpus"),
			cpuSet:    ctx.String("cpuset-cpus"),
		}
		log.Printf("runOpts=%v, subsystemConfig=%v", runOpts, subsystemConfig)

		initCmd, err := os.Readlink("/proc/self/exe")
		if err != nil {
			log.Printf("can't get init command: %v", err)
			return err
		}
		cmd := exec.Command(initCmd, "init")
		cloneFlags := syscall.CLONE_NEWPID | syscall.CLONE_NEWNS | syscall.CLONE_NEWIPC | syscall.CLONE_NEWUTS
		if ctx.String("network") != "" && ctx.String("network") != "host" {
			cloneFlags = cloneFlags | syscall.CLONE_NEWNET
		}
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Cloneflags:   uintptr(cloneFlags),
			Unshareflags: syscall.CLONE_NEWNS,
		}
		if runOpts.createTty {
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}
		readPipe, writePipe, err := os.Pipe()
		if err != nil {
			log.Printf("Create pipe failed: %v", err)
			return err
		}
		cmd.ExtraFiles = []*os.File{readPipe}
		cmd.Dir = makeContainerMergedDir(runOpts.containerName)
		if err := createContainerWorkspace(runOpts); err != nil {
			return err
		}
		if err = cmd.Start(); err != nil {
			log.Printf("can't start command: %v, %v", cmd, err)
			return err
		}

		cgroup := NewCgroup(runOpts.containerId)
		if err := cgroup.Set(subsystemConfig); err != nil {
			return err
		}
		if err := cgroup.Apply(cmd.Process.Pid); err != nil {
			return err
		}
		defer cgroup.Destroy()

		container := Container{
			id: runOpts.containerId,
			pid: cmd.Process.Pid,
		}
		network := ctx.String("net")
		var need_net = false
		if network != "" && network != "host" {
			need_net = true
			if err := Connect(network, &container); err != nil {
				return err
			}
		}

		log.Printf("sending command: %v", runOpts.command)
		writePipe.WriteString(runOpts.command)
		writePipe.Close()

		if runOpts.createTty {
			cmd.Wait()
			if need_net {
				if err := Disconnect(network, container); err != nil {
					return err
				}
			}
			if err := cleanContainerWorkspace(runOpts); err != nil {
				return err
			}
		}

		return nil
	},
}

func createContainerWorkspace(opts RunOptions) error {
	imagePath := makeImagePath(opts.imageName)
	_, err := os.Stat(imagePath)
	if err != nil {
		return err
	}

	mergedDir := makeContainerMergedDir(opts.containerName)
	if err := os.MkdirAll(mergedDir, 0777); err != nil {
		return err
	}
	upperDir := makeContainerUpperDir(opts.containerName)
	if err := os.MkdirAll(upperDir, 0777); err != nil {
		return err
	}
	workDir := makeContainerWorkDir(opts.containerName)
	if err := os.MkdirAll(workDir, 0777); err != nil {
		return err
	}

	if err := syscall.Mount("overlay", mergedDir, "overlay", 0, fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", imagePath, upperDir, workDir)); err != nil {
		return err
	}

	if len(opts.volumes) == 2 {
		if err := os.MkdirAll(opts.volumes[0], 0755); err != nil {
			return err
		}
		containerVolumePath := path.Join(mergedDir, opts.volumes[1])
		if err := os.MkdirAll(containerVolumePath, 0755); err != nil {
			return err
		}
		if err := syscall.Mount(opts.volumes[0], containerVolumePath, "bind", syscall.MS_BIND, ""); err != nil {
			return err
		}
	}

	return nil
}

func cleanContainerWorkspace(opts RunOptions) error {
	if err := syscall.Unmount(makeContainerMergedDir(opts.containerName), syscall.MNT_DETACH); err != nil {
		return err
	}
	if err := os.RemoveAll(makeContainerDir(opts.containerName)); err != nil {
		return err
	}
	return nil
}

