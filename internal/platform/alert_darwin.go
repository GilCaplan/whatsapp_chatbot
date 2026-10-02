package platform

import (
	"fmt"
	"os/exec"
	"strings"
)

// Alert shows a native error dialog (best effort; used when the launcher
// fails and there is no terminal to print to).
func Alert(title, msg string) {
	esc := func(s string) string {
		return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
	}
	script := fmt.Sprintf(`display alert "%s" message "%s" as critical`, esc(title), esc(msg))
	_ = exec.Command("osascript", "-e", script).Run()
}
