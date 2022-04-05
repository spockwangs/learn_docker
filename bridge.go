package main

import (
	"net"
)

type Bridge struct{
}

func (b *Bridge) Name() string {
	return "bridge"
}

func (b *Bridge) Create(subnet, name string) (*Network, error) {
	ip, ipNet, err := net.ParseCIDR(subnet)
	if err != nil {
		return nil, err
	}
	ipNet.IP = ip
	nw := &Network{
		Name: name,
		IpNet: IpNet,
		Driver: b.Name(),
	}

	// Check if the bridge already exists?
	_, err = net.InterfaceByName(nw.Name)
	if err == nil || !strings.Contains(err.Error(), "no such network interface") {
		return nw, err
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
		return fmt.Errorf("iptables failed: output=%v, err=%w", output, err)
	}
	return nw, nil
}
	
func (b *Bridge) Delete(network Network) error {
	bridge, err := netlink.LinkByName(network.Name)
	if err != nil {
		return err
	}
	return netlink.LinkDel(bridge)
}

func (b *Bridge) Connect(network Network, endpoint Endpoint) error {
	bridge, err := netlink.LinkByName(network.Name)
	if err != nil {
		return err
	}

	linkAttr := netlink.NewLinkAttrs()
	linkAttr.Name = endpoint.ID[:5]
	linkAttr.MasterIndex = bridge.Attrs().Index

	endpoint.Device = netlink.Veth{
		LinkAttrs: linkAttr,
		PeerName: "cif-" + endpoint.ID[:5],
	}

	if err = netlink.LinkAdd(&endpoint.Device); err != nil {
		return err
	}
	if err = netlink.LinkSetUp(&endpoint.Device); err != nil {
		return err
	}
	return nil
}
