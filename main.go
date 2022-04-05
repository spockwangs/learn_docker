package main

import (
	"github.com/urfave/cli"
	"log"
	"os"
)

func main() {
	app := cli.NewApp()
	app.Name = "mydocker"
	app.Usage = `mydocker is a simple container`
	app.Commands = []cli.Command{
		initCommand,
		runCommand,
		importCommand,
		commitCommand,
		networkCommand,
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
