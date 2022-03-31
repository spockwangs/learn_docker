package main

import (
	"log"
	"fmt"
	"github.com/urfave/cli"
	"math/rand"
)

type RunOptions struct {
	createTty bool
	containerName string
	containerId string
	imageName string
	command []string
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
		runOpts.command = argArray[1:]
		runOpts.containerName = ctx.String("name")
		runOpts.createTty = ctx.Bool("i")
		runOpts.containerId = makeContainerId()
		if runOpts.containerName == "" {
			runOpts.containerName = runOpts.containerId
		}
		log.Printf("runOpts=%v", runOpts)
		
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
		
func makeContainerId() string {
	const alphanum = "0123456789abcdefghijklmnopqrstuvwxyz"
	const alphanumLen = len(alphanum)
	b := make([]byte, alphanumLen)
	for i := range b {
		b[i] = alphanum[rand.Intn(alphanumLen)]
	}
	return string(b)
}
