package main

import (
	"net"
	"strings"
	"fmt"
	"github.com/vishvananda/netlink"
	"os/exec"
)

type BridgeDriver struct{
}

func (b *BridgeDriver) Name() string {
	return "bridge"
}

func (b *BridgeDriver) Create(subnet, name string) (*Network, error) {
	ip, ipNet, err := net.ParseCIDR(subnet)
	if err != nil {
		return nil, err
	}
	ipNet.IP = ip
	nw := &Network{
		Name: name,
		IpNet: ipNet,
		Driver: b.Name(),
	}

	// Check if the bridge already exists?
	_, err = net.InterfaceByName(nw.Name)
	if err == nil || !strings.Contains(err.Error(), "no such network interface") {
		return nil, fmt.Errorf("the network already exists")
	}
	
	// Create a bridge.
	linkAttr := netlink.NewLinkAttrs()
	linkAttr.Name = nw.Name
	bridge := &netlink.Bridge{LinkAttrs: linkAttr}
	if err := netlink.LinkAdd(bridge); err != nil {
		return nil, fmt.Errorf("can't create bridge `%v`: %w", b.Name(), err)
	}

	// Set IP for the bridge.
	addr := &netlink.Addr{IPNet: ipNet, Peer: ipNet, Label: "", Flags: 0, Scope: 0, Broadcast: nil}
	if err := netlink.AddrAdd(bridge, addr); err != nil {
		return nil, err
	}

	if err := netlink.LinkSetUp(bridge); err != nil {
		return nil, err
	}

	cmd := exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-s", ipNet.String(), "!", "-o", nw.Name, "-j", "MASQUERADE")
	if output, err := cmd.Output(); err != nil {
		return nil, fmt.Errorf("iptables failed: output=%v, err=%w", output, err)
	}

	// Enable forwarding.
	cmd = exec.Command("iptables", "-t", "filter", "-A", "FORWARD", "-i", nw.Name, "!", "-o", nw.Name, "-j", "ACCEPT")
	if output, err := cmd.Output(); err != nil {
		return nil, fmt.Errorf("iptables failed: output=%v, err=%w", output, err)
	}
	cmd = exec.Command("iptables", "-t", "filter", "-A", "FORWARD", "-o", nw.Name, "-j", "ACCEPT")
	if output, err := cmd.Output(); err != nil {
		return nil, fmt.Errorf("iptables failed: output=%v, err=%w", output, err)
	}
	return nw, nil
}
	
func (b *BridgeDriver) Delete(network Network) error {
	bridge, err := netlink.LinkByName(network.Name)
	if err != nil {
		return err
	}
	return netlink.LinkDel(bridge)
}

func (b *BridgeDriver) Connect(network Network, container *Container) error {
	bridge, err := netlink.LinkByName(network.Name)
	if err != nil {
		return err
	}

	linkAttr := netlink.NewLinkAttrs()
	linkAttr.Name = makeVethName(container.id)
	linkAttr.MasterIndex = bridge.Attrs().Index
	container.peerName = "cif-" + linkAttr.Name
	veth := netlink.Veth{
		LinkAttrs: linkAttr,
		PeerName: container.peerName,
	}

	if err = netlink.LinkAdd(&veth); err != nil {
		return err
	}
	if err = netlink.LinkSetUp(&veth); err != nil {
		return err
	}
	return nil
}

func (b *BridgeDriver) Disconnect(container Container) error {
	veth, err := netlink.LinkByName(makeVethName(container.id))
	if err != nil {
		return err
	}
	if err := netlink.LinkSetDown(veth); err != nil {
		return err
	}
	if err := netlink.LinkDel(veth); err != nil {
		return err
	}
	return nil
}
