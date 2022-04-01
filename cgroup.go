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
	cpuShare int
	memory string
	cpuPeriod int
	cpuQuota int
	cpuSet string
	cpus float64
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
		err := Apply(subsys, c.id, pid)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Cgroup) Destroy() error {
	for _, subsys := range subsystems {
		err := Destroy(subsys, c.id)
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

func Apply(sys Subsystem, id string, pid int) error {
	cgroupPath, err := GetCgroupPath(sys.Name(), id)
	if err != nil {
		return err
	}
	if _, err := os.Stat(cgroupPath); err != nil {
		return nil
	}
	
	if err := ioutil.WriteFile(path.Join(cgroupPath, "cgroup.procs"), []byte(strconv.Itoa(pid)), 0644); err != nil {
		return err
	}
	return nil
}

func Destroy(sys Subsystem, id string) error {
	cgroupPath, err := GetCgroupPath(sys.Name(), id)
	if err != nil {
		return err
	}
	return os.RemoveAll(cgroupPath)
}

type Subsystem interface {
	Name() string
	Set(id string, config SubsystemConfig) error
}

var (
	subsystems = []Subsystem{
		&CpuSubsystem{},
		&CpusetSubsystem{},
	}
)

type CpuSubsystem struct {
}

func (c *CpuSubsystem) Name() string {
	return "cpu"
}

func (c *CpuSubsystem) Set(id string, config SubsystemConfig) error {
	if config.cpuShare == 0 && config.cpuPeriod == 0 && config.cpuQuota == 0 && config.cpus == 0 {
		return nil
	}
	if config.cpus != 0 {
		config.cpuPeriod = 1000000
		config.cpuQuota = int(config.cpus * float64(config.cpuPeriod))
	}
	
	cgroupPath, err := GetCgroupPath(c.Name(), id)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cgroupPath, 0755); err != nil {
		return err
	}

	if config.cpuShare != 0 {
		if err := ioutil.WriteFile(path.Join(cgroupPath, "cpu.shares"), []byte(strconv.Itoa(config.cpuShare)), 0644); err != nil {
			return err
		}
	}
	if config.cpuPeriod != 0 {
		if err := ioutil.WriteFile(path.Join(cgroupPath, "cpu.cfs_period_us"), []byte(strconv.Itoa(config.cpuPeriod)), 0644); err != nil {
			return err
		}
	}		
	if config.cpuQuota != 0 {
		if err := ioutil.WriteFile(path.Join(cgroupPath, "cpu.cfs_quota_us"), []byte(strconv.Itoa(config.cpuQuota)), 0644); err != nil {
			return err
		}
	}		
		
	return nil
}

type CpusetSubsystem struct {
}

func (c *CpusetSubsystem) Name() string {
	return "cpuset"
}


func (c *CpusetSubsystem) Set(id string, config SubsystemConfig) error {
	if config.cpuSet == "" {
		return nil
	}

	cgroupPath, err := GetCgroupPath(c.Name(), id)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cgroupPath, 0755); err != nil {
		return err
	}

	if err := ioutil.WriteFile(path.Join(cgroupPath, "cpuset.cpus"), []byte(config.cpuSet), 0644); err != nil {
		return err
	}
	return nil
}
