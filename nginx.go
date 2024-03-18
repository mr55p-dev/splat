package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

type ServiceManager struct {
	// conn *dbus.Conn
	nginxBasePath string
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

func NewServiceManager(ctx context.Context, opts ...ServiceManagerOption) (*ServiceManager, error) {
	// conn, err := dbus.NewSystemdConnection()
	// if err != nil {
	// 	return nil, err
	// }
	manager := &ServiceManager{
		nginxBasePath: "/etc/nginx/conf.d",
		// conn: conn,
	}

	for _, v := range opts {
		v(manager)
	}

	return manager, nil
}

func (sm *ServiceManager) Close() {
	// sm.conn.Close()
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

func (sm *ServiceManager) InstallNignxConfig(ctx context.Context, data []byte, name, env string) error {
	configPath := filepath.Join(sm.nginxBasePath, fmt.Sprintf("splat.%s.%s.conf", name, env))
	f, err := os.Create(configPath)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err != nil {
		return err
	}
	return nil
}

func (sm *ServiceManager) reloadNginx(ctx context.Context) error {
	// _, err := sm.conn.RestartUnit("nginx", "replace", nil)
	// if err != nil {
	// 	return err
	// }
	return nil
}
