package main

import (
	"log"
	"os/exec"
	"github.com/urfave/cli"
	"syscall"
)

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
	if err := syscall.Mount(path, path, "bind", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
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
