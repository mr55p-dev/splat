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

func (sm *ServiceManager) Install(ctx context.Context, data []byte, name, env string) error {
	configPath := filepath.Join(sm.nginxBasePath, fmt.Sprintf("splat.%s.%s.conf", name, env))
	f, err := os.Create(configPath)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err != nil {
		return err
	}
	sm.configPaths = append(sm.configPaths, configPath)
	return nil
}

func (sm *ServiceManager) Start(ctx context.Context) error  { return nil }
func (sm *ServiceManager) Stop(ctx context.Context) error   { return nil }
func (sm *ServiceManager) Reload(ctx context.Context) error { return nil }
