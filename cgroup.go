package main

import (
	"os"
	"bufio"
	"path"
	"strings"
	"io/ioutil"
	"strconv"
)

type Cgroup struct {
	id string
}

type SubsystemConfig struct {
	cpuShare string
	memory string
}

func NewCgroup(id string) *Cgroup {
	return &Cgroup{
		id: id,
	}
}

func (c *Cgroup) Set(config SubsystemConfig) error {
	for _, subsys := range subsystems {
		err := subsys.Set(c.id, config)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Cgroup) Apply(pid int) error {
	for _, subsys := range subsystems {
		err := subsys.Apply(c.id, pid)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Cgroup) Destroy() error {
	for _, subsys := range subsystems {
		err := subsys.Destroy(c.id)
		if err != nil {
			return err
		}
	}
	return nil
}

func GetCgroupPath(subsystemName string, id string) (string, error) {
	root, err := FindCgroupRoot(subsystemName)
	if err != nil {
		return "", err
	}
	return path.Join(root, id), nil
}

func FindCgroupRoot(subsystemName string) (string, error) {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return "", nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		txt := scanner.Text()
		fields := strings.Split(txt, " ")
		for _, opt := range strings.Split(fields[len(fields)-1], ",") {
			if opt == subsystemName {
				return fields[4], nil
			}
		}
	}
	return "", scanner.Err()
}

type Subsystem interface {
	Name() string
	Set(id string, config SubsystemConfig) error
	Apply(id string, pid int) error
	Destroy(id string) error
}

var (
	subsystems = []Subsystem{
		&CpuSubsystem{},
	}
)

type CpuSubsystem struct {
}

func (c *CpuSubsystem) Name() string {
	return "cpu"
}

func (c *CpuSubsystem) Set(id string, config SubsystemConfig) error {
	if config.cpuShare == "" {
		return nil
	}
	
	cgroupPath, err := GetCgroupPath(c.Name(), id)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cgroupPath, 0755); err != nil {
		return nil
	}
	if err := ioutil.WriteFile(path.Join(cgroupPath, "cpu.shares"), []byte(config.cpuShare), 0644); err != nil {
		return err
	}
	return nil
}

func (c *CpuSubsystem) Apply(id string, pid int) error {
	cgroupPath, err := GetCgroupPath(c.Name(), id)
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(path.Join(cgroupPath, "tasks"), []byte(strconv.Itoa(pid)), 0644); err != nil {
		return err
	}
	return nil
}

func (c *CpuSubsystem) Destroy(id string) error {
	cgroupPath, err := GetCgroupPath(c.Name(), id)
	if err != nil {
		return err
	}
	return os.RemoveAll(cgroupPath)
}
