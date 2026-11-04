//go:build !darwin && !windows

package platform

import (
	"log"
	"os/exec"
)

// Alert shows an error dialog with the first tool available (zenity,
// kdialog, then a critical notify-send notification); otherwise it logs.
func Alert(title, msg string) {
	tries := [][]string{
		{"zenity", "--error", "--no-wrap", "--title", title, "--text", msg},
		{"kdialog", "--title", title, "--error", msg},
		{"notify-send", "-a", "WhatsApp Doppel", "-u", "critical", title, msg},
	}
	for _, t := range tries {
		bin, err := exec.LookPath(t[0])
		if err != nil {
			continue
		}
		if exec.Command(bin, t[1:]...).Run() == nil {
			return
		}
	}
	log.Printf("%s: %s", title, msg)
}
