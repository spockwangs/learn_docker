package main

import (
	"log"
	"fmt"
	"github.com/urfave/cli"
)

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

		var argArray []string
		for _, arg := range ctx.Args() {
			argArray = append(argArray, arg)
		}
		imageName := argArray[0]
		command := argArray[1:]

		containerName := ctx.String("name")
		createTty := ctx.Bool("i")
		log.Printf("createTty=%v, containerName=%v, imageName=%v, command=%v", createTty, containerName, imageName, command)
		return nil
	},
}

var initCommand = cli.Command{
	Name: "init",
	Usage: "Not intended for external use",
	Action: func(ctx *cli.Context) error {
		log.Printf("enter initCommand")
		return nil
	},
}
		
