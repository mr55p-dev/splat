package main

import (
	"bytes"
	"context"
	"os/exec"

	"github.com/charmbracelet/log"
)

func (sm *ServiceManager) Reload(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "brew", "services", "restart", "nginx")
	out := new(bytes.Buffer)
	cmd.Stdout = out
	cmd.Stderr = out
	err := cmd.Run()
	if err != nil {
		log.Error("Failed to restart nginx", "error", err.Error())
	} else {
		log.Debug("Restarted nginx")
	}
	return err
}
