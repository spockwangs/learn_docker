package main

import (
	"github.com/urfave/cli"
	"os"
	"log"
)

func main() {
	app := cli.NewApp()
	app.Name = "mydocker"
	app.Usage = `mydocker is a simple container`
	app.Commands = []cli.Command{
		initCommand,
		runCommand,
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
