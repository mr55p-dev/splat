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

	return &DockerEngine{
		docker:           docker,
		dockerLogFile:    dockerLog,
		containerLogFile: appLog,
	}, nil
}

func (engine *DockerEngine) Close() error {
	return engine.docker.Close()
}

func (engine *DockerEngine) SetAuthFromLoginToken(login, serverAddr string) error {
	authConfig := registry.AuthConfig{
		Username:      "AWS",
		Password:      LOGIN_TOKEN,
		ServerAddress: serverAddr,
	}
	tokenJson, err := json.Marshal(&authConfig)
	if err != nil {
		return fmt.Errorf("Error marshalling token to json", err)
	}
	engine.token = base64.URLEncoding.EncodeToString(tokenJson)
	return nil
}

func (engine *DockerEngine) PullImage(ctx context.Context, repo, tag string) error {
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
		fmt.Errorf("Failed to read image data: %s", err.Error())
	}
	log.Debug("Done")
	return nil
}

func (engine *DockerEngine) CreateAndStart(ctx context.Context, image string, replace bool) (string, error) {
	containerName := fmt.Sprintf("%s-rumtine", image)
	containerCreate, err := engine.docker.ContainerCreate(
		ctx,
		&container.Config{
			Image: image,
		},
		nil, nil, nil, containerName,
	)
	if err != nil {
		return containerName, fmt.Errorf("Failed to create container", err)
	}
	log.Info("Created container", "id", containerCreate.ID)
	err = engine.docker.ContainerStart(
		ctx,
		containerCreate.ID,
		container.StartOptions{},
	)
	if err != nil {
		return containerName, fmt.Errorf("Failed to start container", err)
	}

	return containerName, nil
}
