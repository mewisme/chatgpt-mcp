package application

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

func OpenBrowser(raw string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", raw)
	case "darwin":
		command = exec.Command("open", raw)
	default:
		if os.Getenv("WSL_DISTRO_NAME") != "" {
			if _, err := exec.LookPath("explorer.exe"); err == nil {
				command = exec.Command("explorer.exe", raw)
				break
			}
		}
		command = exec.Command("xdg-open", raw)
	}
	if command == nil {
		return fmt.Errorf("no browser opener available")
	}
	return command.Start()
}
