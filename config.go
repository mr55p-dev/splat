package main

import "fmt"

var StartupApps []string = []string{
	// "pagemail.prd.yaml",
	"example.dev.yaml",
}

type RunningAppData struct {
	config        *AppConfig
	engine        *DockerEngine
	uid           string
	containerId   string
	containerName string
	internalPort  string
	status        string
}

var RUNNING_CONTAINER_DATA map[string]*RunningAppData

type AppConfig struct {
	Name      string `config:"name"`
	Container struct {
		Ecr   string `config:"ecr,optional"`
		Image string `config:"image"`
		Tag   string `config:"tag"`
	} `config:"container"`
	ExternalHost  string `config:"nginx.server-name"`
	ContainerPort int    `config:"nginx.container-port"`

	Volumes []struct {
		Name   string `config:"name"`
		Source string `config:"source"`
	} `config:"volumes"`
}

func NewAppContainerData(config *AppConfig) *RunningAppData {
	return &RunningAppData{
		status: "unknown",
		config: config,
		uid: fmt.Sprintf(
			"%s.%s",
			config.Name,
			generateId(),
		),
	}
}
