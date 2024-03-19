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
	"github.com/mr55p-dev/gonk"
)

var StartupApps []string = []string{
	// "pagemail.prd.yaml",
	"example.dev.yaml",
}

func fatal(msg string, err error) {
	log.Fatal(msg, "error", err)
}

type RunningAppData struct {
	config        *AppConfig
	containerId   string
	containerName string
	internalPort  string
}

var RunningContainerData map[string]RunningAppData

type AppConfig struct {
	Name         string `config:"name"`
	Environment  string `config:"environment"`
	ContainerEcr string `config:"container.ecr,optional"`
	ContainerImg string `config:"container.image"`
	ContainerTag string `config:"container.tag"`

	ExternalHost  string `config:"net.external"`
	ContainerPort int    `config:"net.containerPort"`
	LoginToken    string
}

func startupApp(ctx context.Context, config *AppConfig, engine *DockerEngine, serviceManager *ServiceManager) error {
	uid := fmt.Sprintf(
		"%s.%s.%s",
		config.Name,
		config.Environment,
		generateId(),
	)
	// If we have a ECR url pull the image
	if config.ContainerEcr != "" {
		log.Info("Pulling image from AWS", "repo", config.ContainerEcr)
		engine.SetAuthFromLoginToken(config.LoginToken, config.ContainerEcr)
		image := fmt.Sprintf("%s/%s", config.ContainerEcr, config.ContainerImg)
		err := engine.ImagePull(ctx, image, config.ContainerTag)
		if err != nil {
			return err
		}
	}
	log.Info("Using app config")
	log.Printf("%+v", config)

	// Get a port and generate a mapping
	port := generatePort(10000, 10100)
	mainPortMapping := PortMapping{
		ContainerPort: strconv.Itoa(config.ContainerPort),
		HostPort:      strconv.Itoa(port),
		HostAddr:      "0.0.0.0",
		Protocol:      "tcp",
	}
	internalHost := fmt.Sprintf("http://127.0.0.1:%d", port)

	// Setup nginx
	data, err := GenerateNginxConfig(config.ExternalHost, internalHost)
	if err != nil {
		return err
	}

	err = serviceManager.Install(ctx, data, config.Name, config.Environment)
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

	log.Info("Started container", "id", containerName, "port", port)

	RunningContainerData[uid] = RunningAppData{
		config:        config,
		containerName: containerName,
		containerId:   containerId,
		internalPort:  strconv.Itoa(port),
	}

	return nil
}

func main() {
	RunningContainerData = make(map[string]RunningAppData)
	log.SetLevel(log.DebugLevel)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	signals := make(chan os.Signal)
	signal.Notify(
		signals,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGINFO,
		syscall.SIGHUP,
	)
	errs := make(map[string]error)

	LOGIN_TOKEN := os.Getenv("ECR_TOKEN")
	engine, err := NewDockerEnigne(ctx, LOGIN_TOKEN)
	if err != nil {
		panic(err)
	}
	defer engine.Close()

	manager, err := NewServiceManager(ctx, WithNginxPath("/opt/homebrew/etc/nginx/"))
	if err != nil {
		panic(err)
	}
	defer manager.Close()

	go func() {
		for {
			switch <-signals {
			case syscall.SIGINFO:
				// Should print some info out...
				log.Info("Hello siginfo", "containers", RunningContainerData)
			case syscall.SIGHUP:
				// Reload the config
				log.Info("Should be reloading config")
			default:
				cancel()
			}
		}
	}()

	wg := sync.WaitGroup{}
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

			// for now
			appConfig.LoginToken = LOGIN_TOKEN

			err = startupApp(ctx, appConfig, engine, manager)
			if err != nil {
				errs[configPath] = err
			}
		}(configPath)
	}
	wg.Wait()

	for key, val := range errs {
		if val != nil {
			log.Error("Error reading config file", "configFile", key, "error", val.Error())
		}
	}

	// Wait until signals arrive
	<-ctx.Done()
}
