package main

import (
	"context"
	"math/rand"
	"os"
	"strings"
	"sync"
	"syscall"

	"github.com/charmbracelet/log"
)

var chars = []rune("abcdefghijklmnopqrstuvwxyz")

func generateId() string {
	out := strings.Builder{}
	for i := 0; i < 5; i++ {
		idx := rand.Int() % len(chars)
		out.WriteRune(chars[idx])
	}
	return out.String()
}

func generatePort(start, end int) int {
	offset := end - start
	return start + (rand.Int() % offset)
}

type Counter struct {
	sync.Mutex
	count int
}

func (c *Counter) next() int {
	c.Lock()
	defer c.Unlock()
	out := c.count
	c.count++
	return out
}

func NewCounter(start int) *Counter {
	return &Counter{count: start}
}

// listenForSignals handles signal interrupts.
// siginfo prints running container information
// sighup reloads app configs (todo)
// sigint causes the app to gracefully shutdown
func listenForSignals(signals chan os.Signal, cancel context.CancelFunc) {
	for {
		switch <-signals {
		case syscall.SIGINFO:
			// Should print some info out...
			log.Info("Process info")
			for key, info := range RUNNING_CONTAINER_DATA {
				log.Info("", "process", key,
					"Container ID", info.containerId,
					"Container name", info.containerName,
					"Port", info.internalPort,
				)
			}
		case syscall.SIGHUP:
			// Reload the config
			log.Info("Should be reloading config")
		case syscall.SIGINT:
			log.Info("Shutting down...")
			cancel()
		default:
		}
	}
}

func logErrs(errs map[string]error) {
	for key, val := range errs {
		if val != nil {
			log.Error("Error reading config file", "configFile", key, "error", val.Error())
		}
		delete(errs, key)
	}
}
