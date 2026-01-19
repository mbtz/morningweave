package scheduler

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// ControlPaths stores filesystem paths for scheduler coordination.
type ControlPaths struct {
	PID  string
	Stop string
	Log  string
}

// PathsForStorage returns control paths rooted next to the storage file.
func PathsForStorage(storagePath string) ControlPaths {
	dir := strings.TrimSpace(filepath.Dir(storagePath))
	if dir == "" {
		dir = "."
	}
	return ControlPaths{
		PID:  filepath.Join(dir, "scheduler.pid"),
		Stop: filepath.Join(dir, "scheduler.stop"),
		Log:  filepath.Join(dir, "scheduler.log"),
	}
}

// ReadPID returns the PID stored in the pid file.
func ReadPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return 0, errors.New("pid file is empty")
	}
	pid, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid pid value: %w", err)
	}
	return pid, nil
}

// WritePID writes the current process id to the pid file.
func WritePID(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	pid := strconv.Itoa(os.Getpid())
	return os.WriteFile(path, []byte(pid+"\n"), 0o644)
}

// RemovePID removes the pid file if it exists.
func RemovePID(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	_ = os.Remove(path)
}

// RequestStop creates the stop file.
func RequestStop(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte("stop\n"), 0o644)
}

// ClearStop removes the stop file.
func ClearStop(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	_ = os.Remove(path)
}

// StopRequested checks whether the stop file exists.
func StopRequested(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// ProcessRunning reports whether a PID is still running.
func ProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, syscall.Signal(0))
	if err == nil {
		return true
	}
	if errors.Is(err, syscall.EPERM) {
		return true
	}
	return false
}

// SignalStop sends SIGTERM to the provided PID.
func SignalStop(pid int) error {
	if pid <= 0 {
		return errors.New("invalid pid")
	}
	return syscall.Kill(pid, syscall.SIGTERM)
}
