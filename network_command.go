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
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"runtime"
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
				return DeleteNetwork(networkName)
			},
		},
	},
}

func CreateNetwork(driver, subnet, name string) error {
	_, ipNet, err := net.ParseCIDR(subnet)
	if err != nil {
		return err
	}

	d, exist := drivers[driver]
	if !exist {
		return fmt.Errorf("the driver `%v` does not exist", driver)
	}

	ip, err := ipAllocator.Allocate(ipNet)
	if err != nil {
		return err
	}

	ipNet.IP = ip
	network, err := d.Create(ipNet.String(), name)
	if err != nil {
		ipAllocator.Release(ipNet, ip)
		return err
	}
	if err := network.Save(); err != nil {
		ipAllocator.Release(ipNet, ip)
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
	if err := ipAllocator.Release(nw.IpNet, nw.IpNet.IP); err != nil {
		return err
	}
	d, exist := drivers[nw.Driver]
	if !exist {
		return fmt.Errorf("the driver `%v` does not exist", nw.Driver)
	}
	if err := d.Delete(*nw); err != nil {
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
	networkPath := makeNetworkPath(n.Name)
	dir, _ := path.Split(networkPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(networkPath, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0644)
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

const NetworkPath = "/var/run/mydocker/networks/"

func makeNetworkPath(name string) string {
	return path.Join(NetworkPath, name)
}

type NetworkDriver interface {
	Create(subnet string, name string) (*Network, error)
	Delete(network Network) error
	Connect(network Network, container *Container) error
	Disconnect(container Container) error
}

type Endpoint struct {
	id string
	device netlink.Veth
	ip net.IP
	network Network
}

var drivers = map[string]NetworkDriver{
	"bridge": &BridgeDriver{},
}

func Connect(networkName string, container *Container) error {
	// Create a pair of endpoints with the network.
	network, err := NewNetwork(networkName)
	if err != nil {
		return err
	}
	ip, err := ipAllocator.Allocate(network.IpNet)
	if err != nil {
		return err
	}
	defer func() {
		if (err != nil) {
			ipAllocator.Release(network.IpNet, ip)
		}
	}()
	
	container.ip = ip
	if err = drivers[network.Driver].Connect(*network, container); err != nil {
		return err
	}

	// Move one endpoint to the container's network namespace.
	peerLink, err := netlink.LinkByName(container.peerName)
	if err != nil {
		return err
	}
	netFile, err := os.OpenFile(fmt.Sprintf("/proc/%d/ns/net", container.pid), os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer netFile.Close()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err = netlink.LinkSetNsFd(peerLink, int(netFile.Fd())); err != nil {
		return err
	}
	oldNetNs, err := netns.Get()
	if err != nil {
		return err
	}
	defer oldNetNs.Close()
	if err = netns.Set(netns.NsHandle(netFile.Fd())); err != nil {
		return err
	}
	defer netns.Set(oldNetNs)

	ifaceIp := *network.IpNet
	ifaceIp.IP = container.ip
	if err = setInterfaceIp(container.peerName, ifaceIp); err != nil {
		return err
	}
	if err = setInterfaceUp(container.peerName); err != nil {
		return err
	}
	if err = setInterfaceUp("lo"); err != nil {
		return err
	}
	_, ipNet, _ := net.ParseCIDR("0.0.0.0/0")
	defaultRoute := &netlink.Route{
		LinkIndex: peerLink.Attrs().Index,
		Gw: network.IpNet.IP,
		Dst: ipNet,
	}
	if err = netlink.RouteAdd(defaultRoute); err != nil {
		return err
	}

	return nil
}

func Disconnect(networkName string, container Container) error {
	network, err := NewNetwork(networkName)
	if err != nil {
		return err
	}
	if err := drivers[network.Driver].Disconnect(container); err != nil {
		return err
	}
	if err := ipAllocator.Release(network.IpNet, container.ip); err != nil {
		return err
	}
	return nil
}

func setInterfaceIp(linkName string, ipNet net.IPNet) error {
	link, err := netlink.LinkByName(linkName)
	if err != nil {
		return err
	}
	addr := &netlink.Addr{IPNet: &ipNet, Peer: &ipNet, Label: "", Flags: 0, Scope: 0, Broadcast: nil}
	return netlink.AddrAdd(link, addr)
}

func setInterfaceUp(linkName string) error {
	link, err := netlink.LinkByName(linkName)
	if err != nil {
		return err
	}
	return netlink.LinkSetUp(link)
}

