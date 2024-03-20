package main

import (
	"context"
	"fmt"
	"golang.org/x/sync/errgroup"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/charmbracelet/log"
	"github.com/docker/docker/client"
	"github.com/mr55p-dev/gonk"
)

var LOGIN_TOKEN string

const (
	LOG_PATH          = "./logs"
	LOOPBACK_IP       = "127.0.0.1"
	NET_PROTOCOL      = "tcp"
	NGINX_CONFIG_DIR  = "/opt/homebrew/etc/nginx/"
	VOLUME_MOUNT_ROOT = "/volumes"
)

func NewAppContainerData(config *AppConfig) *RunningAppData {
	return &RunningAppData{
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

func pullEcrImage(ctx context.Context, engine *DockerEngine, repo, image, tag string) error {
	log.Info("Pulling image from AWS", "repo", repo)
	engine.SetAuthFromLoginToken(LOGIN_TOKEN, repo)
	image = fmt.Sprintf("%s/%s", repo, image)
	return engine.ImagePull(ctx, image, tag)
}

func startupApp(
	ctx context.Context,
	portCounter *Counter,
	uid, logPath string,
	dockerClient *client.Client,
	serviceManager *ServiceManager,
) error {
	proc, ok := AppContainerData[uid]
	if !ok {
		return fmt.Errorf("process with uid %s not found", uid)
	}

	// start the docker engine client
	engine, err := NewDockerEnigne(
		ctx,
		dockerClient,
		DockerWithLogFiles(logPath, uid),
	)
	if err != nil {
		return err
	}
	proc.engine = engine
	config := proc.config

	// If we have a ECR url pull the image
	if proc.config.Container.Ecr != "" {
		pullEcrImage(
			ctx, engine,
			config.Container.Ecr,
			config.Container.Image,
			config.Container.Tag,
		)
		if err != nil {
			return err
		}
	}
	log.Info("Using app config")

	// Get the default port mapping
	port := portCounter.next()
	mainPortMapping := PortMapping{
		ContainerPort: strconv.Itoa(config.ContainerPort),
		HostPort:      strconv.Itoa(port),
		HostAddr:      LOOPBACK_IP,
		Protocol:      NET_PROTOCOL,
	}
	internalHost := fmt.Sprintf("http://%s:%d", LOOPBACK_IP, port)

	// Get any configured volume mapping
	mainVolumeMapping := VolumeMapping{
		Name:   config.Volumes.Name,
		Source: config.Volumes.Source,
	}

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
	containerId, err := proc.engine.ContainerCreateAndStart(ctx, ContainerCreateAndStartOpts{
		name:     uid,
		image:    config.Container.Image,
		replace:  true,
		networks: []PortMapping{mainPortMapping},
		volumes:  []VolumeMapping{mainVolumeMapping},
	})
	if err != nil {
		return err
	}

	log.Info("Started container",
		"name", uid,
		"id", containerId,
		"port", port,
	)

	proc.containerName = uid
	proc.containerId = containerId
	proc.internalPort = strconv.Itoa(port)
	return nil
}

func main() {
	// Initialize
	LOGIN_TOKEN = os.Getenv("ECR_TOKEN")
	log.SetLevel(log.DebugLevel)
	AppContainerData = make(map[string]*RunningAppData)
	portCounter := NewCounter(10000)
	wg := errgroup.Group{}

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

	// Create a service manager
	manager, err := NewServiceManager(
		ctx,
		WithNginxPath(NGINX_CONFIG_DIR),
	)
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
		path := configPath
		wg.Go(func() error {
			appConfig := new(AppConfig)
			err := gonk.LoadConfig(
				appConfig,
				gonk.FileLoader(path, false),
			)
			if err != nil {
				return err
			}

			proc := NewAppContainerData(appConfig)
			AppContainerData[proc.uid] = proc
			return nil
		})
	}
	if err := wg.Wait(); err != nil {
		panic(err)
	}
	log.Info("Done reading configs")

	// Start the app containers
	for uid := range AppContainerData {
		id := uid
		wg.Go(func() error { // Setup the docker client
			err = startupApp(
				ctx, portCounter, id,
				LOG_PATH, engine, manager,
			)
			if err != nil {
				return err
			}
			return nil
		})
	}
	if err = wg.Wait(); err != nil {
		panic(err)
	}
	log.Info("Done starting apps")

	// Wait until signals arrive
	<-ctx.Done()
	for _, proc := range AppContainerData {
		if proc.engine != nil {
			proc.engine.Close()
		} else {
			log.Warn("No engine found for application", "uid", proc.uid)
		}
	}
}
