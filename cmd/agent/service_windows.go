//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	serviceName    = "InvoicesUpAgent"
	serviceDisplay = "InvoicesUp Connector Agent"
	serviceDesc    = "Mirrors InvoicesUp invoice exports into a local folder for Golden .Net and confirms deliveries."
	logMaxBytes    = 5 << 20 // rotate agent.log past 5 MB
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

	if isService {
		// Redirect logging to a file BEFORE any config/filesystem work, so a
		// startup failure leaves a diagnostic trail (a service has no console).
		setupServiceLog()
		if err := svc.Run(serviceName, &handler{configPath: configPath}); err != nil {
			log.Fatalf("service %s failed: %v", serviceName, err)
		}
		return
	}

	rt, err := prepare(configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	serve(ctx, rt)
}

// setupServiceLog redirects logging to a size-capped file, since a service has
// no console.
func setupServiceLog() {
	dir := programDataDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	path := filepath.Join(dir, "agent.log")
	if fi, err := os.Stat(path); err == nil && fi.Size() > logMaxBytes {
		_ = os.Rename(path, path+".old") // keep one previous log
	}
	if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		log.SetOutput(f)
	}
}

// handler adapts serve() to the Windows Service Control Manager.
type handler struct{ configPath string }

func (h *handler) Execute(_ []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	s <- svc.Status{State: svc.StartPending}

	// Load config inside Execute so a failure is logged and reported to the SCM
	// as a clean start failure (exit code 1) rather than a bare process exit.
	rt, err := prepare(h.configPath)
	if err != nil {
		log.Printf("startup failed: %v", err)
		return false, 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		serve(ctx, rt)
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
		return withService(stopAndWait)
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
		return fmt.Errorf("service %s already exists (uninstall first)", serviceName)
	}

	s, err := m.CreateService(serviceName, exe, mgr.Config{
		DisplayName:  serviceDisplay,
		Description:  serviceDesc,
		StartType:    mgr.StartAutomatic,
		ErrorControl: windows.SERVICE_ERROR_NORMAL,
	}, "run", "-config", configPath)
	if err != nil {
		return err
	}
	defer s.Close()

	// Auto-restart on unexpected exit (crash/panic); reset the failure count
	// after a day of health.
	_ = s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
	}, 86400)

	if err := s.Start("run"); err != nil {
		_ = s.Delete() // roll back the half-install so a retry is clean
		return fmt.Errorf("service registered but failed to start (rolled back): %w", err)
	}
	return nil
}

func removeService() error {
	return withService(func(s *mgr.Service) error {
		_ = stopAndWait(s) // best-effort stop + wait so Delete doesn't race a running process
		return s.Delete()
	})
}

// stopAndWait signals a stop and waits (bounded) for the service to actually
// reach Stopped. An already-stopped service is not an error.
func stopAndWait(s *mgr.Service) error {
	status, err := s.Control(svc.Stop)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
			return nil
		}
		return err
	}
	for i := 0; i < 50 && status.State != svc.Stopped; i++ {
		time.Sleep(200 * time.Millisecond)
		if status, err = s.Query(); err != nil {
			return err
		}
	}
	return nil
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
