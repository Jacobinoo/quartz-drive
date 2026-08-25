package main

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// lock represents an exclusive deployment lock backed by a PID file.
type lock struct {
	path string
}

// acquireLock tries to create an exclusive lock at path.
// If a lock file exists and its process is still alive, it returns an error.
// Stale lock files (process dead) are automatically cleaned up.
func acquireLock(path string) (*lock, error) {
	if data, err := os.ReadFile(path); err == nil {
		// Lock file exists — check if that process is still running
		pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
		if parseErr == nil {
			proc, findErr := os.FindProcess(pid)
			if findErr == nil {
				// Signal 0 checks process existence without sending any signal
				if killErr := proc.Signal(syscall.Signal(0)); killErr == nil {
					return nil, fmt.Errorf(
						"another deployment is already running (PID %d) — "+
							"if this is wrong, delete %s manually",
						pid, path,
					)
				}
			}
		}
		// Stale lock file (process is dead) — remove it
		slog.Warn("Removing stale lock file", "path", path)
		os.Remove(path)
	}

	// Write our own PID as the lock
	content := fmt.Sprintf("%d\n", os.Getpid())
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("creating lock file at %s: %w", path, err)
	}

	slog.Debug("Lock acquired", "path", path, "pid", os.Getpid())
	return &lock{path: path}, nil
}

// release removes the lock file. Safe to call multiple times (idempotent).
func (l *lock) release() {
	if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
		slog.Warn("Failed to release lock file", "path", l.path, "error", err)
	} else {
		slog.Debug("Lock released", "path", l.path)
	}
}
