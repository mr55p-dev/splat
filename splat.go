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
	"syscall"

	"github.com/charmbracelet/log"
	"github.com/mr55p-dev/gonk"
)

var StartupApps []string = []string{
	"pagemail.prd.yaml",
	"example.dev.yaml",
}

func fatal(msg string, err error) {
	log.Fatal(msg, "error", err)
}

type AppConfig struct {
	Name         string `config:"name"`
	Environment  string `config:"environment"`
	EcrRepo      string `config:"ecr.repository"`
	ExternalHost string `config:"net.external"`
	InternalHost string `config:"net.internal"`
	LoginToken   string
}

func (conf *AppConfig) getContainerName() string {
	return fmt.Sprintf("%s.%s", conf.Name, conf.Environment)
}

func (conf *AppConfig) getImageName() string {
	return fmt.Sprintf("%s:%s", conf.Name, "latest")
}

func startupApp(ctx context.Context, config *AppConfig, engine *DockerEngine, serviceManager *ServiceManager) error {
	// Auth with the registry
	engine.SetAuthFromLoginToken(config.LoginToken, config.EcrRepo)
	// Pull the image down
	err := engine.ImagePull(ctx, config.EcrRepo, "latest")
	if err != nil {
		return err
	}

	// Setup nginx
	data, err := GenerateNginxConfig(config.ExternalHost, config.InternalHost)
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
	containerName := fmt.Sprintf("/%s-%s-runtime", config.Name, config.Environment)
	err = engine.ContainerCreateAndStart(ctx, containerName, config.Name, true)
	if err != nil {
		return err
	}

	log.Info("Started container", "id", containerName)

	return nil
}

func main() {
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
				containers := make([]string, len(engine.containers))
				for key := range engine.containers {
					containers = append(containers, key)
				}
				log.Info("Hello siginfo", "containers", containers)
			case syscall.SIGHUP:
				// Reload the config
				log.Info("Should be reloading config")
			default:
				cancel()
			}
		}
	}()

	for _, configPath := range StartupApps {
		appConfig := new(AppConfig)
		err := gonk.LoadConfig(appConfig, gonk.FileLoader(configPath, false))
		if err != nil {
			errs[configPath] = err
			continue
		}

		// for now
		appConfig.LoginToken = LOGIN_TOKEN

		err = startupApp(ctx, appConfig, engine, manager)
		if err != nil {
			errs[configPath] = err
			continue
		}
	}

	for key, val := range errs {
		if val != nil {
			log.Error("Error reading config file", "configFile", key, "error", val.Error())
		}
	}

	// Wait until signals arrive
	<-ctx.Done()
}
