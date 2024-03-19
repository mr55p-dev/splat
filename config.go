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

var AppContainerData map[string]RunningAppData

type AppConfig struct {
	Name         string `config:"name"`
	Environment  string `config:"environment"`
	ContainerEcr string `config:"container.ecr,optional"`
	ContainerImg string `config:"container.image"`
	ContainerTag string `config:"container.tag"`

	ExternalHost  string `config:"net.external"`
	ContainerPort int    `config:"net.containerPort"`
}
