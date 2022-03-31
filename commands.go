package main

import (
	"log"
	"fmt"
	"github.com/urfave/cli"
	"math/rand"
	"os"
	"strings"
	"os/exec"
	"syscall"
	"io/ioutil"
)

type RunOptions struct {
	createTty bool
	containerName string
	containerId string
	imageName string
	command string
}
	
var runCommand = cli.Command{
	Name: "run",
	Usage: `Run a container: mydocker [-i] image command`,
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name: "i",
			Usage: "enable tty",
		},
		cli.StringFlag{
			Name: "name",
			Usage: "container name",
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
		log.Printf("runOpts=%v", runOpts)

		readPipe, writePipe, err := os.Pipe()
		if err != nil {
			log.Printf("Create pipe failed: %v", err)
			return err
		}

		initCmd, err := os.Readlink("/proc/self/exe")
		if err != nil {
			log.Printf("can't get init command: %v", err)
			return err
		}

		if err := createContainerWorkspace(runOpts); err != nil {
			return err
		}

		cmd := exec.Command(initCmd, "init")
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Cloneflags: syscall.CLONE_NEWUTS | syscall.CLONE_NEWPID | syscall.CLONE_NEWNS | syscall.CLONE_NEWNET | syscall.CLONE_NEWIPC,
			Unshareflags: syscall.CLONE_NEWNS,
		}
		if runOpts.createTty {
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}
		cmd.ExtraFiles = []*os.File{readPipe}
		cmd.Dir = makeContainerMergedDir(runOpts.containerName)
		if err = cmd.Start(); err != nil {
			log.Printf("can't start command: %v, %v", cmd, err)
			return err
		}

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
	Name: "init",
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
		
func makeContainerId() string {
	const alphanum = "0123456789abcdefghijklmnopqrstuvwxyz"
	const alphanumLen = len(alphanum)
	b := make([]byte, alphanumLen)
	for i := range b {
		b[i] = alphanum[rand.Intn(alphanumLen)]
	}
	return string(b)
}

const (
	ContainersDir string = "/var/run/mydocker/containers"
	ImageDir string = "/var/run/mydocker/images"
)

func makeContainerDir(containerName string) string {
	return fmt.Sprintf("%s/%s", containerName)
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
	if err := syscall.Mount("proc", "/proc", "proc", syscall.MS_NOEXEC | syscall.MS_NOSUID | syscall.MS_NODEV, ""); err != nil {
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
	if err := syscall.Unmount(oldRootFilename, syscall.MNT_DETACH); err != nil {
		return err
	}
	return os.Remove(oldRootFilename)
}
