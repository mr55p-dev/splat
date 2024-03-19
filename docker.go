package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/log"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
)

type DockerEngine struct {
	docker           *client.Client
	dockerLogFile    io.Writer
	containerLogFile io.Writer
	token            string
	containers       map[string]bool
}

func NewDockerEnigne(ctx context.Context, token string) (*DockerEngine, error) {

	dockerLog, err := os.Create("doker-engine.log")
	if err != nil {
		return nil, fmt.Errorf("Error creating log file: %s", err.Error())
	}
	appLog, err := os.Create("app.log")
	if err != nil {
		return nil, fmt.Errorf("Error creating app log: %s", err.Error())
	}

	// pull the latest container image from ECR
	docker, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, fmt.Errorf("Failed to start the docker engine client: %s", err.Error())
	}

	engine := &DockerEngine{
		docker:           docker,
		dockerLogFile:    dockerLog,
		containerLogFile: appLog,
		containers:       make(map[string]bool),
	}

	return engine, nil
}

func (engine *DockerEngine) Close() error {
	for key := range engine.containers {
		engine.ContainerStopAndRemove(context.Background(), key)
	}
	return engine.docker.Close()
}

func (engine *DockerEngine) SetAuthFromLoginToken(login, serverAddr string) error {
	authConfig := registry.AuthConfig{
		Username:      "AWS",
		Password:      login,
		ServerAddress: serverAddr,
	}
	tokenJson, err := json.Marshal(&authConfig)
	if err != nil {
		return fmt.Errorf("Error marshalling token to json: %s", err.Error())
	}
	engine.token = base64.URLEncoding.EncodeToString(tokenJson)
	return nil
}

func (engine *DockerEngine) ImagePull(ctx context.Context, repo, tag string) error {
	img, err := engine.docker.ImagePull(
		ctx,
		fmt.Sprintf("%s:%s", repo, tag),
		types.ImagePullOptions{
			RegistryAuth: engine.token,
		},
	)
	if err != nil {
		return fmt.Errorf("Failed to pull image: %s", err.Error())
	}
	defer img.Close()
	_, err = io.Copy(engine.dockerLogFile, img)
	log.Info("Pulling image")

	if err != nil {
		return fmt.Errorf("Failed to read image data: %s", err.Error())
	}
	log.Debug("Done pulling image")
	return nil
}

func (engine *DockerEngine) ContainerStop(ctx context.Context, name string) error {
	timer := 10
	err := engine.docker.ContainerStop(ctx, name, container.StopOptions{
		Timeout: &timer,
	})
	if err != nil {
		log.Error("Failed to stop container")
		return err
	}
	log.Debug("Stopped container", "containerId", name)
	return nil
}

func (engine *DockerEngine) ContainerRemove(ctx context.Context, containerName string) error {
	err := engine.docker.ContainerRemove(ctx, containerName, container.RemoveOptions{
		RemoveVolumes: false,
	})
	if err != nil {
		log.Error("Failed to remove container")
		return err
	}
	log.Debug("Removed container", "containerId", containerName)
	delete(engine.containers, containerName)
	return nil
}

func (engine *DockerEngine) ContainerStopAndRemove(ctx context.Context, containerName string) error {
	containers, err := engine.docker.ContainerList(ctx, container.ListOptions{
		Size: false,
		All:  true,
	})
	if err != nil {
		log.Error("Failed to list container")
		return err
	}

	for _, container := range containers {
		for _, name := range container.Names {
			if name == containerName {
				log.Debug(
					"Found existing container",
					"containerId",
					container.ID,
					"containerState",
					container.State,
				)
				if container.State == "running" {
					err = engine.ContainerStop(ctx, name)
					if err != nil {
						return err
					}
				}
				err = engine.ContainerRemove(ctx, name)
				if err != nil {
					return err
				}
				return nil
			}
		}
	}
	log.Debug("No existing container")
	return nil
}

func (engine *DockerEngine) ContainerCreateAndStart(ctx context.Context, name, image string, replace bool) error {
	log.Info("Operating container", "containerId", name, "imageId", image)
	if replace {
		err := engine.ContainerStopAndRemove(ctx, name)
		if err != nil {
			return err
		}
	}
	containerCreate, err := engine.docker.ContainerCreate(
		ctx,
		&container.Config{
			Image: image,
		},
		nil, nil, nil, name,
	)
	if err != nil {
		return fmt.Errorf("Failed to create container: %s", err.Error())
	}
	engine.containers[name] = true
	log.Info("Created container", "id", containerCreate.ID)
	err = engine.docker.ContainerStart(
		ctx,
		containerCreate.ID,
		container.StartOptions{},
	)
	if err != nil {
		return fmt.Errorf("Failed to start container: %s", err)
	}

	return nil
}

func getContainerId(imageName, environment string) string {
	return fmt.Sprintf("/%s-%s-runtime", imageName, environment)
}
