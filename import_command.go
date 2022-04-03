package main

import (
	"github.com/urfave/cli"
)

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
