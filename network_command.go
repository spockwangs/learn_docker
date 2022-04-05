package main

import (
	"github.com/urfave/cli"
	"net"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"path"
	"io/ioutil"
	"encoding/json"
	"text/tabwriter"
)

var networkCommand = cli.Command{
	Name: "network",
	Usage: "manage network",
	Subcommands: []cli.Command{
		{
			Name: "create",
			Usage: "create a container network",
			UsageText: `mydocker network create OPTIONS NETWORK`,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name: "driver",
					Usage: "network driver",
				},
				cli.StringFlag{
					Name: "subnet",
					Usage: "subnet cidr",
				},
			},
			Action: func(ctx *cli.Context) error {
				if len(ctx.Args()) != 1 {
					return fmt.Errorf("missing network name")
				}

				return CreateNetwork(ctx.String("driver"), ctx.String("subnet"), ctx.Args().Get(0))
			},
		},
		{
			Name: "list",
			Usage: "list container network",
			UsageText: `mydocker network list`,
			Action: func(ctx *cli.Context) error {
				networks, err := ListNetwork()
				if err != nil {
					return err
				}

				writer := tabwriter.NewWriter(os.Stdout, 12, 1, 3, ' ', 0)
				fmt.Fprint(writer, "NAME\tIpNet\tDriver\n")
				for _, nw := range networks {
					fmt.Fprintf(writer, "%s\t%s\t%s\n", nw.Name, nw.IpNet.String(), nw.Driver)
				}
				if err := writer.Flush(); err != nil {
					return err
				}
				return nil
			},
		},
		{
			Name: "remove",
			Usage: "remove container network",
			UsageText: `mydocker network remove NETWORK`,
			Action: func(ctx *cli.Context) error {
				if len(ctx.Args()) != 1 {
					return fmt.Errorf("missing network name")
				}
				networkName := ctx.Args().Get(0)
				nw, err := NewNetwork(networkName)
				if err != nil {
					return err
				}
				return nw.Remove()
			},
		},
	},
}

func CreateNetwork(driver, subnet, name string) error {
	_, ipNet, err := net.ParseCIDR(subnet)
	if err != nil {
		return err
	}
	ip, err := ipAllocator.Allocate(ipNet)
	if err != nil {
		return err
	}
	d, err := drivers[driver]
	if err != nil {
		return fmt.Errorf("the driver `%v` does not exist", driver)
	}
	network, err := d.Create(ipNet, name)
	if err != nil {
		return err
	}
	if err := network.Save(); err != nil {
		return err
	}
	return nil
}

func ListNetwork() ([]Network, error) {
	if _, err := os.Stat(NetworkPath); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(NetworkPath, 0755); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	var networks []Network
	err := filepath.Walk(NetworkPath, func(networkPath string, info os.FileInfo, err error) error {
		if strings.HasSuffix(networkPath, "/") {
			return nil
		}

		_, name := path.Split(networkPath)
		nw, err := NewNetwork(name)
		if err != nil {
			return err
		}
		networks = append(networks, *nw)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return networks, nil
}

func DeleteNetwork(networkName string) error {
	nw, err := NewNetwork(networkName)
	if err != nil {
		return err
	}
	if err := ipAllocator.Release(nw.IpNet, &nw.IpNet.IP); err != nil {
		return err
	}
	d, err := drivers[nw.Driver]
	if err != nil {
		return err
	}
	if err := d.Delete(nw); err != nil {
		return err
	}
	return nw.Remove()
}

type Network struct{
	Name string
	IpNet *net.IPNet
	Driver string
}

func NewNetwork(name string) (*Network, error) {
	path := makeNetworkPath(name)
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	jsonStr, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}

	nw := &Network{}
	if err := json.Unmarshal(jsonStr, nw); err != nil {
		return nil, err
	}
	return nw, nil
}

func (n *Network) Save() error {
	path := makeNetworkPath(n.Name)
	file, err := os.OpenFile(path, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	jsonStr, err := json.Marshal(n)
	if err != nil {
		return err
	}
	if _, err := file.Write(jsonStr); err != nil {
		return err
	}
	return nil
}

func (n *Network) Remove() error {
	networkPath := makeNetworkPath(n.Name)
	return os.RemoveAll(networkPath)
}

const NetworkPath = "/var/run/mydocker/networks"

func makeNetworkPath(name string) string {
	return path.Join(NetworkPath, name)
}

type NetworkDriver interface {
	Name() string
	Create(subnet string, name string) (*Network, error)
	Delete(network Network) error
}

var drivers := map[string]NetworkDriver{
	&Bridge{}
}
