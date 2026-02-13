package notify

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// Notify sends a desktop notification. On macOS uses osascript, on Linux uses notify-send.
func Notify(title, message string) error {
	switch runtime.GOOS {
	case "darwin":
		return notifyMac(title, message)
	case "linux":
		return notifyLinux(title, message)
	default:
		// No-op on other platforms
		return nil
	}
}

func notifyMac(title, message string) error {
	script := fmt.Sprintf(`display notification "%s" with title "%s"`, message, title)
	cmd := exec.Command("osascript", "-e", script)
	return cmd.Run()
}

func notifyLinux(title, message string) error {
	cmd := exec.Command("notify-send", title, message)
	return cmd.Run()
}

// NotifyIfEnabled sends a notification only if enabled
func NotifyIfEnabled(enabled bool, title, message string) error {
	if !enabled {
		return nil
	}
	return Notify(title, message)
}
