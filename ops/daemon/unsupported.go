//go:build !darwin && !linux

package daemon

import "fmt"

// Install is not supported on this platform.
func Install(_ ServiceConfig) error {
	return fmt.Errorf("background auto-capture needs macOS (launchd) or Linux (systemd); on this platform run `mom watch` in the foreground to capture, and `mom vault fold` to synthesize")
}

// Uninstall is not supported on this platform.
func Uninstall(_ ServiceConfig) error {
	return fmt.Errorf("background recording not supported on this platform")
}

// Status is not supported on this platform.
func Status(_ ServiceConfig) (*Health, error) {
	return &Health{Platform: "unsupported"}, nil
}

// InstallGlobal is not supported on this platform.
func InstallGlobal(_ GlobalServiceConfig) error {
	return fmt.Errorf("background auto-capture needs macOS (launchd) or Linux (systemd); on this platform run `mom watch` in the foreground to capture, and `mom vault fold` to synthesize")
}

// UninstallGlobal is not supported on this platform.
func UninstallGlobal() error {
	return fmt.Errorf("background recording not supported on this platform")
}

// RestartGlobal is not supported on this platform.
func RestartGlobal() error {
	return fmt.Errorf("background recording not supported on this platform")
}

// StatusGlobal is not supported on this platform.
func StatusGlobal() (*Health, error) {
	return &Health{Platform: "unsupported"}, nil
}

// GlobalDaemonFile is not supported on this platform.
func GlobalDaemonFile() (string, error) {
	return "", nil
}

// CleanupLegacy is not supported on this platform.
func CleanupLegacy(_ string) error {
	return nil
}
