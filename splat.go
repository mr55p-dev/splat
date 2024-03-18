/*
Tasks:
- Install app
- Upgrade app
- Delete app

## Install app:
1. Pull docker image from ECR
2. Install nginx config
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

	"github.com/charmbracelet/log"
	"github.com/mr55p-dev/gonk"
)

func fatal(msg string, err error) {
	log.Fatal(msg, "error", err)
}

type AppConfig struct {
	Name         string `config:"name"`
	Environment  string `config:"environment"`
	EcrRepo      string `config:"ecr.repository"`
	ExternalHost string `config:"net.external"`
	InternalHost string `config:"net.internal"`
}

func (conf *AppConfig) getContainerName() string {
	return fmt.Sprintf("%s.%s", conf.Name, conf.Environment)
}

func (conf *AppConfig) getImageName() string {
	return fmt.Sprintf("%s:%s", conf.Name, "latest")
}

var LOGIN_TOKEN string

func init() {
	LOGIN_TOKEN = os.Getenv("ECR_TOKEN")
}

func InstallApplicationFromConfig(ctx context.Context, config *AppConfig, engine *DockerEngine, serviceManager *ServiceManager) error {
	// Auth with the registry
	engine.SetAuthFromLoginToken(LOGIN_TOKEN, config.EcrRepo)
	// Pull the image down
	err := engine.PullImage(ctx, config.EcrRepo, "latest")
	if err != nil {
		return err
	}

	// Setup nginx
	data, err := GenerateNginxConfig(config.ExternalHost, config.InternalHost)
	if err != nil {
		return err
	}
	err = serviceManager.InstallNignxConfig(ctx, data, config.Name, config.Environment)
	if err != nil {
		return err
	}
	err = serviceManager.reloadNginx(ctx)
	if err != nil {
		return err
	}

	// Launch a docker container
	containerId, err := engine.CreateAndStart(ctx, config.Name, false)
	if err != nil {
		return err
	}

	log.Info("Started container", "id", containerId)

	return nil
}

func main() {
	log.SetLevel(log.DebugLevel)

	// load configs from somewhere?
	appConf := AppConfig{
		Name: "pagemail",
	}
	err := gonk.LoadConfig(
		&appConf,
		gonk.FileLoader("app.prd.yaml", false),
	)
	if err != nil {
		fatal("Failed to load app.yaml", err)
	}
	ctx := context.TODO()

	engine, err := NewDockerEnigne(ctx, LOGIN_TOKEN)
	if err != nil {
		fatal("Failed to start docker engine", err)
	}

	manager, err := NewServiceManager(ctx, WithNginxPath("./nginx"))
	if err != nil {
		fatal("Failed to start service manager", err)
	}

	err = InstallApplicationFromConfig(ctx, &appConf, engine, manager)
	if err != nil {
		fatal("Failed to install service", err)
	}

	log.Info("Done")
}
