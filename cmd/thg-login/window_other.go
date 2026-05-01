//go:build !windows

package main

import "fmt"

func findLocalChromeProcessID(port int) int {
	return 0
}

func hideLocalChromeProcessWindow(pid int) error {
	return fmt.Errorf("native window hide is not implemented on this OS")
}

func showLocalChromeProcessWindow(pid int) error {
	return fmt.Errorf("native window show is not implemented on this OS")
}
