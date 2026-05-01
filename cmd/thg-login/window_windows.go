//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

var (
	user32                     = syscall.NewLazyDLL("user32.dll")
	procEnumWindows            = user32.NewProc("EnumWindows")
	procGetWindowThreadProcess = user32.NewProc("GetWindowThreadProcessId")
	procShowWindow             = user32.NewProc("ShowWindow")
	procSetWindowPos           = user32.NewProc("SetWindowPos")
)

const (
	swHide        = 0
	swShownormal  = 1
	swMinimize    = 6
	swRestore     = 9
	swpNoSize     = 0x0001
	swpNoActivate = 0x0010
	swpNoZOrder   = 0x0004
)

func findLocalChromeProcessID(port int) int {
	if port <= 0 {
		return 0
	}
	out, err := exec.Command("netstat", "-ano", "-p", "tcp").Output()
	if err != nil {
		return 0
	}
	needleA := fmt.Sprintf("127.0.0.1:%d", port)
	needleB := fmt.Sprintf("0.0.0.0:%d", port)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "LISTENING") {
			continue
		}
		if !strings.Contains(line, needleA) && !strings.Contains(line, needleB) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		pid, err := strconv.Atoi(fields[len(fields)-1])
		if err == nil && pid > 0 {
			return pid
		}
	}
	return 0
}

func hideLocalChromeProcessWindow(pid int) error {
	windows := topLevelWindowsForPID(pid)
	if len(windows) == 0 {
		return fmt.Errorf("no visible Chrome window found for pid %d", pid)
	}
	for _, hwnd := range windows {
		procShowWindow.Call(hwnd, uintptr(swMinimize))
		procSetWindowPos.Call(hwnd, 0, int32ToUintptr(-32000), int32ToUintptr(-32000), 0, 0, uintptr(swpNoSize|swpNoZOrder|swpNoActivate))
		procShowWindow.Call(hwnd, uintptr(swHide))
	}
	return nil
}

func showLocalChromeProcessWindow(pid int) error {
	windows := topLevelWindowsForPID(pid)
	if len(windows) == 0 {
		return fmt.Errorf("no Chrome window found for pid %d", pid)
	}
	for _, hwnd := range windows {
		procShowWindow.Call(hwnd, uintptr(swShownormal))
		procSetWindowPos.Call(hwnd, 0, int32ToUintptr(80), int32ToUintptr(60), 0, 0, uintptr(swpNoSize|swpNoZOrder))
		procShowWindow.Call(hwnd, uintptr(swRestore))
	}
	return nil
}

func topLevelWindowsForPID(pid int) []uintptr {
	if pid <= 0 {
		return nil
	}
	var hwnds []uintptr
	cb := syscall.NewCallback(func(hwnd uintptr, lparam uintptr) uintptr {
		var windowPID uint32
		procGetWindowThreadProcess.Call(hwnd, uintptr(unsafe.Pointer(&windowPID)))
		if int(windowPID) != pid {
			return 1
		}
		hwnds = append(hwnds, hwnd)
		return 1
	})
	procEnumWindows.Call(cb, 0)
	return hwnds
}

func int32ToUintptr(value int32) uintptr {
	return uintptr(uint32(value))
}
