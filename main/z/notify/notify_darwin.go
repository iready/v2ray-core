//go:build darwin

package notify

import (
	"fmt"
	"os/exec"
	"strings"
)

func show(title, body string) error {
	script := fmt.Sprintf(`display notification %s with title %s`, appleQuote(body), appleQuote(title))
	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func appleQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
