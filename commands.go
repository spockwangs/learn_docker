package main

import (
	"fmt"
	"github.com/urfave/cli"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"path"
)

type RunOptions struct {
	createTty     bool
	containerName string
	containerId   string
	imageName     string
	command       string
	volumes []string
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
		runOpts.volumes = strings.Split(ctx.String("v"), ":")
		if len(runOpts.volumes) != 2 {
			return fmt.Errorf("bad volumes")
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
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Cloneflags:   syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS | syscall.CLONE_NEWNET | syscall.CLONE_NEWIPC,
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

		log.Printf("sending command: %v", runOpts.command)
		writePipe.WriteString(runOpts.command)
		writePipe.Close()

		if runOpts.createTty {
			cmd.Wait()
			if err := cleanContainerWorkspace(runOpts); err != nil {
				return err
			}
		}

		return nil
	},
}

var initCommand = cli.Command{
	Name:  "init",
	Usage: "Not intended for external use",
	Action: func(ctx *cli.Context) error {
		log.Printf("enter initCommand")
		cmdArray, err := readCommand()
		if err != nil {
			return err
		}

		if err := setUpMountPoints(); err != nil {
			return err
		}

		path, err := exec.LookPath(cmdArray[0])
		if err != nil {
			return err
		}
		log.Printf("command path=%v", path)
		if err := syscall.Exec(path, cmdArray, os.Environ()); err != nil {
			log.Printf("can't exec: %v", err)
			return err
		}

		return nil
	},
}

var importCommand = cli.Command{
	Name: "import",
	Usage: "import a tarball to create an image",
	UsageText: "mydocker import FILE IMAGE",
	Action: func(ctx *cli.Context) error {
		if len(ctx.Args()) != 2 {
			return fmt.Errorf("missing tarball path and/or image name")
		}
		tarballPath := ctx.Args().Get(0)
		imageName := ctx.Args().Get(1)
		imagePath := makeImagePath(imageName)
		if err := os.MkdirAll(imagePath, 0755); err != nil {
			return err
		}
		if _, err := exec.Command("tar", "-xvf", tarballPath, "-C", imagePath).CombinedOutput(); err != nil {
			return err
		}
		return nil
	},
}

var commitCommand = cli.Command{
	Name: "commit",
	Usage: "commit the changes of the container to an new image",
	UsageText: `mydocker commit CONTAINER IMAGE`,
	Action: func(ctx *cli.Context) error {
		if len(ctx.Args()) != 2 {
			return fmt.Errorf("missing container name and/or image name")
		}

		containerName := ctx.Args().Get(0)
		imageName := ctx.Args().Get(1)
		containerPath := makeContainerMergedDir(containerName)
		if _, err := os.Stat(containerPath); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("the container `%v` does not exist", containerName)
			}
			return err
		}
		imagePath := makeImagePath(imageName)
		if _, err := os.Stat(imagePath); err == nil {
			return fmt.Errorf("the image `%v` already exits; please change a name", imageName)
		}
		if err := os.MkdirAll(imagePath, 0755); err != nil {
			return err
		}
		if _, err := exec.Command("sh", "-c", fmt.Sprintf("cp -a %s/* %s", containerPath, imagePath)).CombinedOutput(); err != nil {
			return err
		}
		return nil
	},
}

func makeContainerId() string {
	const alphanum = "0123456789abcdefghijklmnopqrstuvwxyz"
	const alphanumLen = len(alphanum)
	rand.Seed(time.Now().UnixNano())
	b := make([]byte, alphanumLen)
	for i := range b {
		b[i] = alphanum[rand.Intn(alphanumLen)]
	}
	return string(b)
}

const (
	ContainersDir string = "/var/run/mydocker/containers"
	ImageDir      string = "/var/run/mydocker/images"
)

func makeContainerDir(containerName string) string {
	return fmt.Sprintf("%s/%s", ContainersDir, containerName)
}

func makeContainerMergedDir(containerName string) string {
	path := fmt.Sprintf("%s/%s/merged", ContainersDir, containerName)
	return path
}

func makeContainerUpperDir(containerName string) string {
	path := fmt.Sprintf("%s/%s/upper", ContainersDir, containerName)
	return path
}

func makeContainerWorkDir(containerName string) string {
	path := fmt.Sprintf("%s/%s/work", ContainersDir, containerName)
	return path
}

func readCommand() ([]string, error) {
	const ReadPipe = uintptr(3)
	pipe := os.NewFile(ReadPipe, "pipe")
	defer pipe.Close()
	msg, err := ioutil.ReadAll(pipe)
	if err != nil {
		return nil, err
	}
	s := string(msg)
	cmdArray := strings.Split(s, " ")
	if len(cmdArray) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	return cmdArray, nil
}

func createContainerWorkspace(opts RunOptions) error {
	// Extract the image.
	imagePath := makeImagePath(opts.imageName)
	_, err := os.Stat(imagePath)
	if err != nil {
		return err
	}
	if _, err := exec.Command("tar", "-xvf", fmt.Sprintf("%s.tar", imagePath), "-C", imagePath).CombinedOutput(); err != nil {
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

func makeImagePath(imageName string) string {
	return fmt.Sprintf("%s/%s", ImageDir, imageName)
}

func setUpMountPoints() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	if err := pivotRoot(cwd); err != nil {
		return err
	}
	if err := syscall.Mount("proc", "/proc", "proc", syscall.MS_NOEXEC|syscall.MS_NOSUID|syscall.MS_NODEV, ""); err != nil {
		log.Printf("can't mount proc: %v", err)
		return err
	}
	if err := syscall.Mount("tmpfs", "/dev", "tmpfs", syscall.MS_NOSUID|syscall.MS_STRICTATIME, "mode=755"); err != nil {
		log.Printf("can't mount tmpfs: %v", err)
		return err
	}
	return nil
}

func pivotRoot(path string) error {
	if err := syscall.Mount(path, path, "bind", syscall.MS_BIND, ""); err != nil {
		return err
	}
	oldRootFilename := "old_root"
	oldRoot := fmt.Sprintf("%s/%s", path, oldRootFilename)
	if err := os.MkdirAll(oldRoot, 0777); err != nil {
		return err
	}
	if err := syscall.PivotRoot(path, oldRoot); err != nil {
		return err
	}
	if err := syscall.Chdir("/"); err != nil {
		return err
	}
	// if err := syscall.Unmount(oldRootFilename, syscall.MNT_DETACH); err != nil {
	// 	return err
	// }
	return os.Remove(oldRootFilename)
}
