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
		cmd.Dir = makeContainerCwd(runOpts.containerName)
		if err = os.MkdirAll(cmd.Dir, 0777); err != nil {
			log.Printf("can't make directory `%v`: %v", cmd.Dir, err)
			return err
		}
		if err = cmd.Start(); err != nil {
			log.Printf("can't start command: %v, %v", cmd, err)
			return err
		}

		log.Printf("sending command: %v", runOpts.command)
		writePipe.WriteString(runOpts.command)
		writePipe.Close()

		if runOpts.createTty {
			cmd.Wait()
			// TODO: 清理容器的目录
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
		
		path, err := exec.LookPath(cmdArray[0])
		if err != nil {
			return err
		}
		if err := syscall.Exec(path, cmdArray, os.Environ()); err != nil {
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

func makeContainerCwd(containerName string) string {
	path := fmt.Sprintf("%s/%s", ContainersDir, containerName)
	return path
}

func readCommand() ([]string, error) {
	pipe := os.NewFile(uintptr(3), "pipe")
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
