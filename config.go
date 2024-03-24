package main

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

var AppContainerData map[string]*RunningAppData

type AppConfig struct {
	Name      string `config:"name"`
	Container struct {
		Ecr   string `config:"ecr,optional"`
		Image string `config:"image"`
		Tag   string `config:"tag"`
	} `config:"container"`
	ExternalHost  string `config:"net.external"`
	ContainerPort int    `config:"net.containerPort"`

	Volumes []struct {
		Name   string `config:"name"`
		Source string `config:"source"`
	} `config:"volumes"`
}
