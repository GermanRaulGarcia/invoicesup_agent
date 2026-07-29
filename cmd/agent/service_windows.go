//go:build windows

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	serviceName    = "InvoicesUpAgent"
	serviceDisplay = "InvoicesUp Connector Agent"
	serviceDesc    = "Mirrors InvoicesUp invoice exports into a local folder for Golden .Net and confirms deliveries."
)

// defaultConfigPath puts machine-wide service config under ProgramData, which is
// writable and outside Program Files.
func defaultConfigPath() string {
	return filepath.Join(programDataDir(), "config.json")
}

func programDataDir() string {
	base := os.Getenv("ProgramData")
	if base == "" {
		base = `C:\ProgramData`
	}
	return filepath.Join(base, "InvoicesUp")
}

// runAgent runs as a Windows service when launched by the SCM, otherwise in the
// foreground (for `agent.exe run` from a console).
func runAgent(configPath string) {
	isService, err := svc.IsWindowsService()
	if err != nil {
		log.Fatalf("cannot determine session type: %v", err)
	}

	rt, err := prepare(configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if isService {
		setupServiceLog()
		if err := svc.Run(serviceName, &handler{rt: rt}); err != nil {
			log.Fatalf("service %s failed: %v", serviceName, err)
		}
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	serve(ctx, rt)
}

// setupServiceLog redirects logging to a file, since a service has no console.
func setupServiceLog() {
	if err := os.MkdirAll(programDataDir(), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(programDataDir(), "agent.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err == nil {
		log.SetOutput(f)
	}
}

// handler adapts serve() to the Windows Service Control Manager.
type handler struct{ rt *runtime }

func (h *handler) Execute(_ []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	s <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		serve(ctx, h.rt)
		close(done)
	}()

	s <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			s <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			s <- svc.Status{State: svc.StopPending}
			cancel()
			<-done
			s <- svc.Status{State: svc.Stopped}
			return false, 0
		}
	}
	return false, 0
}

func controlService(cmd, configPath string) error {
	switch cmd {
	case "install":
		return installService(configPath)
	case "uninstall":
		return removeService()
	case "start":
		return withService(func(s *mgr.Service) error { return s.Start("run") })
	case "stop":
		return stopService()
	}
	return fmt.Errorf("unknown service command %q", cmd)
}

func installService(configPath string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	if s, err := m.OpenService(serviceName); err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists", serviceName)
	}

	s, err := m.CreateService(serviceName, exe, mgr.Config{
		DisplayName: serviceDisplay,
		Description: serviceDesc,
		StartType:   mgr.StartAutomatic,
	}, "run", "-config", configPath)
	if err != nil {
		return err
	}
	defer s.Close()

	return s.Start("run")
}

func removeService() error {
	return withService(func(s *mgr.Service) error {
		// Best-effort stop before delete; ignore "not running" errors.
		_, _ = s.Control(svc.Stop)
		return s.Delete()
	})
}

func stopService() error {
	return withService(func(s *mgr.Service) error {
		_, err := s.Control(svc.Stop)
		return err
	})
}

// withService opens the service and runs fn against it.
func withService(fn func(*mgr.Service) error) error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("service %s is not installed: %w", serviceName, err)
	}
	defer s.Close()

	return fn(s)
}
