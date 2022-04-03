package main

import (
	"github.com/urfave/cli"
	"fmt"
	"os"
    "os/exec"
)

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
