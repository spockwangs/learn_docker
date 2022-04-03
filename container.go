package main

import (
	"math/rand"
)

func makeContainerId() string {
	const alphanum = "0123456789abcdefghijklmnopqrstuvwxyz"
	const alphanumLen = len(alphanum)
	rand.Seed(time.Now().UnixNano())
	b := make([]byte, alphanumLen)
	for i := range b {
		b[i] = alphanum[rand.Intn(alphanumLen)]
	}
	return string(b)
}

const (
	ContainersDir string = "/var/run/mydocker/containers"
	ImageDir      string = "/var/run/mydocker/images"
)

func makeContainerDir(containerName string) string {
	return fmt.Sprintf("%s/%s", ContainersDir, containerName)
}

func makeContainerMergedDir(containerName string) string {
	path := fmt.Sprintf("%s/%s/merged", ContainersDir, containerName)
	return path
}

func makeContainerUpperDir(containerName string) string {
	path := fmt.Sprintf("%s/%s/upper", ContainersDir, containerName)
	return path
}

func makeContainerWorkDir(containerName string) string {
	path := fmt.Sprintf("%s/%s/work", ContainersDir, containerName)
	return path
}

func makeImagePath(imageName string) string {
	return fmt.Sprintf("%s/%s", ImageDir, imageName)
}
