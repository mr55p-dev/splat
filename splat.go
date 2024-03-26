package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"golang.org/x/sync/errgroup"

	"github.com/charmbracelet/log"
	"github.com/docker/docker/client"
	"github.com/mr55p-dev/gonk"
)

var LOGIN_TOKEN string

type startupOptions struct {
	portCounter    *Counter
	uid            string
	logPath        string
	dockerClient   *client.Client
	serviceManager *ServiceManager
}

const (
	LOG_PATH           = "./logs"
	LOOPBACK_IP        = "127.0.0.1"
	NET_PROTOCOL       = "tcp"
	NGINX_CONFIG_DIR   = "/opt/homebrew/etc/nginx/servers/"
	VOLUME_TARGET_ROOT = "/volumes"
	VOLUME_SOURCE_ROOT = "."
)

func pullEcrImage(ctx context.Context, engine *DockerEngine, repo, image, tag string) error {
	log.Info("Pulling image from AWS", "repo", repo)
	engine.SetAuthFromLoginToken(LOGIN_TOKEN, repo)
	image = fmt.Sprintf("%s/%s", repo, image)
	return engine.ImagePull(ctx, image, tag)
}

func LoadFromConfigPath(path string) error {
	appConfig := new(AppConfig)
	configFile, err := gonk.NewYamlLoader(path)
	if err != nil {
		return err
	}
	err = gonk.LoadConfig(appConfig, configFile)
	if err != nil {
		return err
	}

	proc := NewAppContainerData(appConfig)
	AppContainerData[proc.uid] = proc
	return nil

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
		return fmt.Errorf("process with uid %s not found", opts.uid)
	}

	// start the docker engine client
	engine, err := NewDockerEnigne(
		ctx,
		opts.dockerClient,
		DockerWithLogFiles(opts.logPath, opts.uid),
		DockerWithVolumeRoot(VOLUME_TARGET_ROOT),
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
	port := opts.portCounter.next()
	mainPortMapping := PortMapping{
		ContainerPort: strconv.Itoa(config.ContainerPort),
		HostPort:      strconv.Itoa(port),
		HostAddr:      LOOPBACK_IP,
		Protocol:      NET_PROTOCOL,
	}
	internalHost := fmt.Sprintf("http://%s:%d", LOOPBACK_IP, port)

	// Get any configured volume mapping
	var volumes []VolumeMapping
	for _, vol := range config.Volumes {
		if !filepath.IsLocal(vol.Source) {
			return fmt.Errorf("Volume sources must be absolute paths: %s", vol.Source)
		}
		volumes = append(volumes, VolumeMapping{
			Name:   vol.Name,
			Source: filepath.Join(VOLUME_SOURCE_ROOT, vol.Source),
		})
	}

	// Setup nginx
	nginxData, err := GenerateNginxConfig(config.ExternalHost, internalHost)
	if err != nil {
		return err
	}
	err = opts.serviceManager.Install(ctx, nginxData, config.Name)
	if err != nil {
		return err
	}
	err = opts.serviceManager.Reload(ctx)
	if err != nil {
		return err
	}

	// Launch a docker container
	containerId, err := proc.engine.ContainerCreateAndStart(ctx, ContainerCreateAndStartOpts{
		name:     opts.uid,
		image:    config.Container.Image,
		replace:  true,
		networks: []PortMapping{mainPortMapping},
		volumes:  volumes,
	})
	if err != nil {
		return err
	}

	log.Info("Started container",
		"name", opts.uid,
		"id", containerId,
		"port", port,
	)

	proc.containerName = opts.uid
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
	wg.SetLimit(-1)

	// Ensure the context is always cancelled
	ctx, cancel := context.WithCancel(context.Background())

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
		log.Error(err)
		os.Exit(1)
	}
	defer manager.Close()

	// Setup the docker engine
	engine, err := NewDockerClient(ctx)
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}
	defer engine.Close()

	// Load the app configs into the data struct
	for _, configPath := range StartupApps {
		path := configPath
		wg.Go(func() error { return LoadFromConfigPath(path) })
	}
	if err := wg.Wait(); err != nil {
		panic(err)
	}
	log.Info("Done reading configs")

	// Start the app containers
	for uid := range AppContainerData {
		id := uid
		wg.Go(func() error { // Setup the docker client
			err = startupApp(ctx, &startupOptions{
				portCounter:    portCounter,
				uid:            id,
				logPath:        LOG_PATH,
				dockerClient:   engine,
				serviceManager: manager,
			})
			if err != nil {
				return err
			}
			return nil
		})
	}
	if err = wg.Wait(); err != nil {
		log.Error(err)
		cancel()
		goto exit
	}
	log.Info("Done starting apps")

	// Wait until signals arrive
	<-ctx.Done()
exit:
	for _, proc := range AppContainerData {
		if proc.engine != nil {
			proc.engine.Close()
		} else {
			log.Warn("No engine found for application", "uid", proc.uid)
		}
	}
}
