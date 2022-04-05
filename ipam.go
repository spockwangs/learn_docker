package main

import (
	"path"
	"net"
	"io/ioutil"
	"encoding/json"
	"strings"
	"os"
)

type IPAM struct{
}

var ipAllocator = &IPAM{}

const IpamPath = "/var/run/mydocker/ipam.json"

func (ipam *IPAM) Allocate(subnet *net.IPNet) (ip net.IP, err error) {
	subnets, err := loadSubnets()
	if err != nil {
		return
	}
	
	_, subnet, _ = net.ParseCIDR(subnet.String())
	one, size := subnet.Mask.Size()

	if _, exist := subnets[subnet.String()]; !exist {
		subnets[subnet.String()] = strings.Repeat("0", 1 << uint8(size - one))
	}

	for c := range(subnets[subnet.String()]) {
		if subnets[subnet.String()][c] == '0' {
			ipalloc := []byte(subnets[subnet.String()])
			ipalloc[c] = '1'
			subnets[subnet.String()] = string(ipalloc)
			ip = subnet.IP
			for t := uint(4); t > 0; t-=1 {
				[]byte(ip)[4-t] += uint8(c >> ((t - 1) * 8))
			}
			ip[3]+=1
			break
		}
	}
	return
}

func (ipam *IPAM) Release(subnet *net.IPNet, ip *net.IP) error {
	subnets, err := loadSubnets()
	if err != nil {
		return err
	}

	_, subnet, _ = net.ParseCIDR(subnet.String())
	c := 0
	releaseIP := ip.To4()
	releaseIP[3]-=1
	for t := uint(4); t > 0; t-=1 {
		c += int(releaseIP[t-1] - subnet.IP[t-1]) << ((4-t) * 8)
	}

	ipalloc := []byte(subnets[subnet.String()])
	ipalloc[c] = '0'
	subnets[subnet.String()] = string(ipalloc)
	return storeSubnets(subnets)
}

// subnet => allocation bits
type SubnetsConfig = map[string]string

func loadSubnets() (subnets SubnetsConfig, err error) {
	subnets = make(SubnetsConfig)
	if _, err = os.Stat(IpamPath); err != nil {
		if os.IsNotExist(err) {
			err = nil
		}
		return
	}

	file, err := os.Open(IpamPath)
	if err != nil {
		return
	}
	defer file.Close()
	jsonStr, err := ioutil.ReadAll(file)
	if err != nil {
		return
	}
	err = json.Unmarshal(jsonStr, subnets)
	return
}

func storeSubnets(subnets SubnetsConfig) error {
	dir, _ := path.Split(IpamPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	file, err := os.OpenFile(IpamPath, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	jsonStr, err := json.Marshal(subnets)
	if err != nil {
		return err
	}
	_, err = file.Write(jsonStr)
	return err
}
