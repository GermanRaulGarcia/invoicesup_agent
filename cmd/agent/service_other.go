//go:build !windows

package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
)

// defaultConfigPath is the working directory on non-Windows (dev) platforms.
func defaultConfigPath() string { return "config.json" }

// runAgent runs the poll loop in the foreground until interrupted.
func runAgent(configPath string) {
	rt, err := prepare(configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serve(ctx, rt)
}

// controlService is a no-op off Windows: the service commands are Windows-only.
func controlService(string, string) error {
	return errors.New("service commands (install/uninstall/start/stop) are only supported on Windows")
}
