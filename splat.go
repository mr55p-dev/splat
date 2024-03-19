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
	LOG_PATH         = "./logs"
	LOOPBACK_IP      = "127.0.0.1"
	NET_PROTOCOL     = "tcp"
	NGINX_CONFIG_DIR = "/opt/homebrew/etc/nginx/"
)

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
	if proc.config.ContainerEcr != "" {
		log.Info("Pulling image from AWS", "repo", config.ContainerEcr)
		proc.engine.SetAuthFromLoginToken(LOGIN_TOKEN, config.ContainerEcr)
		image := fmt.Sprintf("%s/%s", config.ContainerEcr, config.ContainerImg)
		err := proc.engine.ImagePull(ctx, image, config.ContainerTag)
		if err != nil {
			return err
		}
	}
	log.Info("Using app config")

	port := portCounter.next()
	mainPortMapping := PortMapping{
		ContainerPort: strconv.Itoa(config.ContainerPort),
		HostPort:      strconv.Itoa(port),
		HostAddr:      LOOPBACK_IP,
		Protocol:      NET_PROTOCOL,
	}
	internalHost := fmt.Sprintf("http://%s:%d", LOOPBACK_IP, port)

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
		name:       uid,
		image:      config.ContainerImg,
		replace:    true,
		networkMap: []PortMapping{mainPortMapping},
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
	AppContainerData = make(map[string]RunningAppData)
	log.SetLevel(log.DebugLevel)
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
		wg.Go(func() error {
			appConfig := new(AppConfig)
			err := gonk.LoadConfig(
				appConfig,
				gonk.FileLoader(configPath, false),
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
		wg.Go(func() error { // Setup the docker client
			err = startupApp(
				ctx, portCounter, uid,
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
	for _, info := range AppContainerData {
		info.engine.Close()
	}
}
