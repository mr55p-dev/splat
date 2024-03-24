package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/charmbracelet/log"
)

type ServiceManager struct {
	nginxBasePath string
	configPaths   []string
}

type NginxConfigData struct {
	Host NginxHost
}

type NginxHost struct {
	External string
	Internal string
}

type ServiceManagerOption func(*ServiceManager)

func WithNginxPath(path string) ServiceManagerOption {
	return func(sm *ServiceManager) {
		sm.nginxBasePath = path
	}
}

func NewServiceManager(ctx context.Context, options ...ServiceManagerOption) (*ServiceManager, error) {
	manager := &ServiceManager{
		nginxBasePath: "/etc/nginx/conf.d",
	}

	for _, option := range options {
		option(manager)
	}

	return manager, nil
}

func (sm *ServiceManager) Close() {
	for _, file := range sm.configPaths {
		err := os.RemoveAll(file)
		if err != nil {
			log.Error("Error cleaning up", "configFile", file)
		}
	}
	sm.Reload(context.Background())
}

func GenerateNginxConfig(externalHost, internalHost string) ([]byte, error) {
	output := new(bytes.Buffer)
	templ := template.Must(template.ParseFiles("templates/nginx.conf.gotempl"))
	err := templ.Execute(output, NginxConfigData{
		Host: NginxHost{
			External: externalHost,
			Internal: internalHost,
		},
	})
	return output.Bytes(), err
}

func (sm *ServiceManager) Install(ctx context.Context, data []byte, name string) error {
	configFileName := fmt.Sprintf("splat.%s.conf", name)
	configPath := filepath.Join(sm.nginxBasePath, configFileName)
	f, err := os.Create(configPath)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err != nil {
		return err
	}
	sm.configPaths = append(sm.configPaths, configPath)
	log.Debug("Created nginx config file", "name", configFileName, "dir", sm.nginxBasePath)
	return nil
}
