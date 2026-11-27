//go:build !windows

package main

import (
	"os"
	"syscall"
	"time"
)

func waitProcessExit(pid int, timeout time.Duration) bool {
	if pid <= 0 {
		return true
	}
	deadline := time.Now().Add(timeout)
	for {
		var status syscall.WaitStatus
		reaped, err := syscall.Wait4(pid, &status, syscall.WNOHANG, nil)
		if err == nil && reaped == pid {
			return true
		}
		if err == syscall.ECHILD {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func killBrowserProcess(process *os.Process, pid int) {
	if pid > 0 {
		if pgid, err := syscall.Getpgid(pid); err == nil && pgid == pid {
			if err := syscall.Kill(-pid, syscall.SIGKILL); err == nil {
				return
			}
		}
	}
	if process != nil {
		_ = process.Kill()
	}
}
