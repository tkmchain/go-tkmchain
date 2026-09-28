//go:build !emailgui

package main

import (
	"fmt"
	"os/exec"
	"runtime"
)

func runWindow(url string) error {
	var command string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		command, args = "open", []string{url}
	case "windows":
		command, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		command, args = "xdg-open", []string{url}
	}
	if _, err := exec.LookPath(command); err != nil {
		return fmt.Errorf("cannot open a browser (%s is not installed): %w", command, err)
	}
	return exec.Command(command, args...).Start()
}
