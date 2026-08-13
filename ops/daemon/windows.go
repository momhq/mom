//go:build windows

package daemon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// Windows has no equivalent of launchd's "run this binary directly as my
// service" contract: a process only counts as a service once it calls
// svc.Run, which blocks on the Service Control Manager's dispatcher. Rather
// than teach `mom watch` itself about the SCM (which would mean wiring
// windows/svc through the watch command and every layer above it), the
// service registered here re-execs the SAME stable binary as a plain child
// process (`mom watch --global`) and supervises it — restarting it if it
// exits, and killing it on Stop/Shutdown. This is the "child launch" shape:
// the only SCM-aware code lives in this file, behind RunWindowsServiceIfNeeded
// (called once from cmd/mom's main, a no-op on every other platform).
const (
	globalServiceName  = "MomWatch"
	serviceDisplayName = "MOM Watch Daemon"
	serviceDescription = "Runs `mom watch` in the background to capture harness transcripts into the MOM Ledger."
)

func init() {
	RunWindowsServiceIfNeeded = runWindowsServiceIfNeeded
}

func serviceName(hash string) string {
	return globalServiceName + "-" + hash
}

// runWindowsServiceIfNeeded hands control to the SCM dispatcher when this
// process was launched by it (svc.IsAnInteractiveSession reports false).
// If it turns out we weren't actually started by SCM, svc.Run fails fast
// and execution falls through to the normal CLI path.
func runWindowsServiceIfNeeded() {
	interactive, err := svc.IsAnInteractiveSession()
	if err != nil || interactive {
		return
	}
	if err := svc.Run(globalServiceName, &watchServiceHandler{}); err == nil {
		os.Exit(0)
	}
}

// watchServiceHandler implements svc.Handler by supervising `mom watch
// --global` as a child process for the lifetime of the service.
type watchServiceHandler struct{}

func (h *watchServiceHandler) Execute(_ []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	s <- svc.Status{State: svc.StartPending}

	exePath, err := os.Executable()
	if err != nil {
		s <- svc.Status{State: svc.Stopped}
		return true, 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go superviseWatch(ctx, exePath, done)

	s <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

loop:
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				s <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				s <- svc.Status{State: svc.StopPending}
				cancel()
				break loop
			}
		case <-done:
			// The supervised loop gave up on its own — stop so SCM's
			// failure-recovery policy (if configured) can restart the process.
			break loop
		}
	}

	<-done
	s <- svc.Status{State: svc.Stopped}
	return false, 0
}

// superviseWatch runs `<exePath> watch --global` for as long as ctx is
// alive, restarting it with a short backoff if it exits unexpectedly —
// the Windows analog of launchd's KeepAlive / systemd's Restart=always.
// ponytail: fixed 1s restart delay + doubling-on-failed-start backoff, no
// jitter or crash-loop circuit breaker; revisit if the service starts
// hot-looping in the field.
func superviseWatch(ctx context.Context, exePath string, done chan<- struct{}) {
	defer close(done)

	logPath := ""
	if logsDir, err := GlobalLogsDir(); err == nil {
		logPath = filepath.Join(logsDir, "watch-daemon.log")
	}

	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for ctx.Err() == nil {
		cmd := exec.Command(exePath, "watch", "--global")
		var logFile *os.File
		if logPath != "" {
			if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
				logFile = f
				cmd.Stdout = f
				cmd.Stderr = f
			}
		}

		if err := cmd.Start(); err != nil {
			if logFile != nil {
				_ = logFile.Close()
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, maxBackoff)
			continue
		}
		backoff = time.Second

		waitErr := make(chan error, 1)
		go func() { waitErr <- cmd.Wait() }()

		select {
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			<-waitErr
			if logFile != nil {
				_ = logFile.Close()
			}
			return
		case <-waitErr:
			if logFile != nil {
				_ = logFile.Close()
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

// ── Service Control Manager helpers ─────────────────────────────────────────

func installService(name, binary string, args []string, displayName string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connecting to service manager: %w", err)
	}
	defer func() { _ = m.Disconnect() }()

	// Idempotent like launchd's unload-before-overwrite and systemd's
	// enable --now on rewritten units: stop and delete any prior
	// registration before recreating, so config/binary-path changes apply.
	if s, err := m.OpenService(name); err == nil {
		_, _ = s.Control(svc.Stop)
		waitForStop(s, 10*time.Second)
		_ = s.Delete()
		_ = s.Close()
	}

	cfg := mgr.Config{
		StartType:   mgr.StartAutomatic,
		DisplayName: displayName,
		Description: serviceDescription,
	}
	s, err := m.CreateService(name, binary, cfg, args...)
	if err != nil {
		return fmt.Errorf("creating service %s: %w", name, err)
	}
	defer s.Close()

	if err := s.Start(); err != nil {
		return fmt.Errorf("starting service %s: %w", name, err)
	}
	return nil
}

func removeService(name string) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connecting to service manager: %w", err)
	}
	defer func() { _ = m.Disconnect() }()

	s, err := m.OpenService(name)
	if err != nil {
		return nil // already gone
	}
	defer s.Close()

	_, _ = s.Control(svc.Stop)
	waitForStop(s, 10*time.Second)
	if err := s.Delete(); err != nil {
		return fmt.Errorf("deleting service %s: %w", name, err)
	}
	return nil
}

func serviceInstalled(name string) (bool, error) {
	m, err := mgr.Connect()
	if err != nil {
		return false, err
	}
	defer func() { _ = m.Disconnect() }()
	s, err := m.OpenService(name)
	if err != nil {
		return false, nil
	}
	_ = s.Close()
	return true, nil
}

func serviceRunning(name string) (bool, error) {
	m, err := mgr.Connect()
	if err != nil {
		return false, err
	}
	defer func() { _ = m.Disconnect() }()
	s, err := m.OpenService(name)
	if err != nil {
		return false, err
	}
	defer s.Close()
	status, err := s.Query()
	if err != nil {
		return false, err
	}
	return status.State == svc.Running, nil
}

func waitForStop(s *mgr.Service, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, err := s.Query()
		if err != nil || status.State == svc.Stopped {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// ── ServiceConfig interface (per-project) ───────────────────────────────────

// Install creates and starts a per-project Windows service running `mom
// watch`. Windows registers one service per project — unlike launchd/
// systemd's daemon+sweep-timer pair, there is no fallback sweep timer here
// (that would need Task Scheduler wiring); closing the keepalive gap only
// requires the persistent watcher.
func Install(cfg ServiceConfig) error {
	hash := ProjectHash(cfg.ProjectDir)
	return installService(serviceName(hash), cfg.MomBinary, []string{"watch"},
		serviceDisplayName+" ("+hash+")")
}

// Uninstall stops and removes the per-project Windows service.
func Uninstall(cfg ServiceConfig) error {
	hash := ProjectHash(cfg.ProjectDir)
	return removeService(serviceName(hash))
}

// Status reports whether the per-project Windows service is running.
func Status(cfg ServiceConfig) (*Health, error) {
	hash := ProjectHash(cfg.ProjectDir)
	name := serviceName(hash)
	h := &Health{
		Platform: "windows-service",
		Services: []ServiceHealth{{DaemonLabel: name}},
	}
	if running, err := serviceRunning(name); err == nil {
		h.Services[0].DaemonRunning = running
	}
	return h, nil
}

// ── Global daemon ────────────────────────────────────────────────────────────

// GlobalDaemonFile has no real file to point at on Windows (services live
// in the SCM database, not a unit file on disk) — it returns the service
// name as a stable, human-readable identifier. Callers that need to detect
// installation should use GlobalDaemonInstalled instead of stat-ing this.
func GlobalDaemonFile() (string, error) {
	return globalServiceName, nil
}

// GlobalDaemonInstalled reports whether the global watch Windows service is
// registered with the Service Control Manager.
func GlobalDaemonInstalled() (bool, string, error) {
	installed, err := serviceInstalled(globalServiceName)
	if err != nil {
		return false, "", fmt.Errorf("querying service manager: %w", err)
	}
	if !installed {
		return false, "Windows service " + globalServiceName + " is not installed", nil
	}
	return true, "Windows service " + globalServiceName, nil
}

// InstallGlobal creates and starts the single global Windows service.
func InstallGlobal(cfg GlobalServiceConfig) error {
	return installService(globalServiceName, cfg.MomBinary, []string{"watch", "--global"}, serviceDisplayName)
}

// UninstallGlobal stops and removes the global Windows service.
func UninstallGlobal() error {
	return removeService(globalServiceName)
}

// RestartGlobal restarts the global service in place (stop, then start
// again against the same registered binary path). Since the service always
// points at the stable path (~/.mom/bin/mom, i.e. cfg.MomBinary), restarting
// re-execs whatever binary is on disk now — see the launchd variant for why
// this must be a restart, not an uninstall/reinstall cycle.
func RestartGlobal() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connecting to service manager: %w", err)
	}
	defer func() { _ = m.Disconnect() }()

	s, err := m.OpenService(globalServiceName)
	if err != nil {
		return fmt.Errorf("opening service %s: %w", globalServiceName, err)
	}
	defer s.Close()

	_, _ = s.Control(svc.Stop)
	waitForStop(s, 10*time.Second)
	if err := s.Start(); err != nil {
		return fmt.Errorf("restarting service %s: %w", globalServiceName, err)
	}
	return nil
}

// StatusGlobal reports whether the global Windows service is running.
func StatusGlobal() (*Health, error) {
	h := &Health{
		Platform: "windows-service",
		Services: []ServiceHealth{{DaemonLabel: globalServiceName}},
	}
	if running, err := serviceRunning(globalServiceName); err == nil {
		h.Services[0].DaemonRunning = running
	}
	return h, nil
}

// CleanupLegacy removes a stray per-project Windows service for projectDir,
// if one was ever installed (mirrors the launchd/systemd migration to a
// single global daemon).
func CleanupLegacy(projectDir string) error {
	hash := ProjectHash(projectDir)
	return removeService(serviceName(hash))
}
