/*
Tasks:
- Install app
- Upgrade app
- Delete app

## Install app:
1. ~~Pull docker image from ECR~~
2. ~~Install nginx config~~
3. Reload nginx unit
4. Start docker container

## Upgrade app
1. Pull docker image from ECR
2. Kill existing docker container
3. Start new docker container

## Delete app
1. Stop docker container
2. Delete nginx config
*/
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"

	"github.com/charmbracelet/log"
	"github.com/docker/docker/client"
	"github.com/mr55p-dev/gonk"
)

var LOGIN_TOKEN string
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

func NewAppContainerData(config *AppConfig) RunningAppData {
	return RunningAppData{
		status: "unknown",
		config: config,
		uid: fmt.Sprintf(
			"%s.%s.%s",
			config.Name,
			config.Environment,
			generateId(),
		),
	}
}

func startupApp(
	ctx context.Context,
	portCounter *Counter,
	uid string,
	dockerClient *client.Client,
	serviceManager *ServiceManager,
) error {
	data, ok := AppContainerData[uid]
	if !ok {
		return fmt.Errorf("process with uid %s not found", uid)
	}
	engine, err := NewDockerEnigne(
		ctx,
		dockerClient,
		DockerWithLogFiles("./logs", uid),
	)
	if err != nil {
		return err
	}

	data.engine = engine
	config := data.config

	// If we have a ECR url pull the image
	if config.ContainerEcr != "" {
		log.Info("Pulling image from AWS", "repo", config.ContainerEcr)
		data.engine.SetAuthFromLoginToken(LOGIN_TOKEN, config.ContainerEcr)
		image := fmt.Sprintf("%s/%s", config.ContainerEcr, config.ContainerImg)
		err := engine.ImagePull(ctx, image, config.ContainerTag)
		if err != nil {
			return err
		}
	}
	log.Info("Using app config")
	log.Printf("%+v", config)

	port := portCounter.next()
	mainPortMapping := PortMapping{
		ContainerPort: strconv.Itoa(config.ContainerPort),
		HostPort:      strconv.Itoa(port),
		HostAddr:      "0.0.0.0",
		Protocol:      "tcp",
	}
	internalHost := fmt.Sprintf("http://127.0.0.1:%d", port)

	// Setup nginx
	nginxData, err := GenerateNginxConfig(config.ExternalHost, internalHost)
	if err != nil {
		return err
	}

	err = serviceManager.Install(ctx, nginxData, config.Name, config.Environment)
	if err != nil {
		return err
	}

	err = serviceManager.Reload(ctx)
	if err != nil {
		return err
	}

	// Launch a docker container
	containerName := fmt.Sprintf("/splat-%s-%s-runtime", config.Name, config.Environment)
	containerId, err := engine.ContainerCreateAndStart(ctx, ContainerCreateAndStartOpts{
		name:       containerName,
		image:      config.ContainerImg,
		replace:    true,
		networkMap: []PortMapping{mainPortMapping},
	})
	if err != nil {
		return err
	}

	log.Info("Started container",
		"name", containerName,
		"id", containerId,
		"port", port,
	)

	AppContainerData[uid] = RunningAppData{
		engine:        engine,
		config:        config,
		containerName: containerName,
		containerId:   containerId,
		internalPort:  strconv.Itoa(port),
	}

	return nil
}

func listenForSignals(signals chan os.Signal, cancel context.CancelFunc) {
	for {
		switch <-signals {
		case syscall.SIGINFO:
			// Should print some info out...
			log.Info("Process info")
			for key, info := range AppContainerData {
				log.Info("", "process", key,
					"Container ID", info.containerId,
					"Container name", info.containerName,
					"Port", info.internalPort,
				)
			}
		case syscall.SIGHUP:
			// Reload the config
			log.Info("Should be reloading config")
		default:
			cancel()
		}
	}
}

func logErrs(errs map[string]error) {
	for key, val := range errs {
		if val != nil {
			log.Error("Error reading config file", "configFile", key, "error", val.Error())
		}
		delete(errs, key)
	}
}

func main() {
	// Initialize
	LOGIN_TOKEN = os.Getenv("ECR_TOKEN")
	AppContainerData = make(map[string]RunningAppData)
	log.SetLevel(log.DebugLevel)
	portCounter := NewCounter(10000)
	wg := sync.WaitGroup{}

	// Ensure the context is always cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal listener
	signals := make(chan os.Signal)
	signal.Notify(
		signals,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGINFO,
		syscall.SIGHUP,
	)
	go listenForSignals(signals, cancel)
	errs := make(map[string]error)

	// Create a service manager
	manager, err := NewServiceManager(ctx, WithNginxPath("/opt/homebrew/etc/nginx/"))
	if err != nil {
		panic(err)
	}
	defer manager.Close()

	// Setup the docker engine
	engine, err := NewDockerClient(ctx)
	if err != nil {
		panic(err)
	}
	defer engine.Close()

	// Load the app configs into the data struct
	for _, configPath := range StartupApps {
		wg.Add(1)
		go func(configPath string) {
			defer wg.Done()
			appConfig := new(AppConfig)
			err := gonk.LoadConfig(
				appConfig,
				gonk.FileLoader(configPath, false),
			)
			if err != nil {
				errs[configPath] = err
				return
			}

			proc := NewAppContainerData(appConfig)
			AppContainerData[proc.uid] = proc

		}(configPath)
	}
	wg.Wait()
	logErrs(errs)
	log.Info("Done reading configs")

	// Start the app containers
	for uid := range AppContainerData {
		wg.Add(1)
		go func(uid string) { // Setup the docker client
			defer wg.Done()
			err = startupApp(ctx, portCounter, uid, engine, manager)
			if err != nil {
				errs[uid] = err
				return
			}
		}(uid)
	}
	wg.Wait()
	logErrs(errs)
	log.Info("Done starting apps")

	// Wait until signals arrive
	<-ctx.Done()
	for _, info := range AppContainerData {
		info.engine.Close()
	}
}
