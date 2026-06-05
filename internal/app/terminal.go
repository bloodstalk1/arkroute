package app

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"
)

type terminalRow struct {
	Label string
	Value string
}

func setupReadyMessage(host string, port int, token string, configPath string) string {
	return terminalMessage("setup panel", []terminalRow{
		{Label: "status", Value: "running"},
		{Label: "panel", Value: fmt.Sprintf("http://%s:%d/setup#setup_token=%s", host, port, token)},
		{Label: "config", Value: configPath},
	}, "Choose a provider, save config, then activate clients from the panel.")
}

func panelReadyMessage(page string, url string, configPath string) string {
	title := "control panel"
	if page == "setup" {
		title = "setup panel"
	}
	return terminalMessage(title, []terminalRow{
		{Label: "status", Value: "ready"},
		{Label: "panel", Value: url},
		{Label: "config", Value: configPath},
	}, "")
}

func serveReadyMessage(addr string, configPath string, logPath string) string {
	return terminalMessage("gateway", []terminalRow{
		{Label: "status", Value: "listening"},
		{Label: "url", Value: "http://" + addr},
		{Label: "config", Value: configPath},
		{Label: "traces", Value: logPath},
	}, "")
}

func providerSetupGuidanceMessage() string {
	return terminalRowsMessage("provider", []terminalRow{
		{Label: "status", Value: "not configured"},
		{Label: "next", Value: "arkroute setup"},
	})
}

func terminalMessage(title string, rows []terminalRow, footer string) string {
	var b strings.Builder
	b.WriteString(">_ arkroute\n")
	b.WriteString("   terminal portal gateway\n\n")
	b.WriteString(terminalRowsMessage(title, rows))
	if footer != "" {
		b.WriteString("\n")
		b.WriteString(footer)
		b.WriteString("\n")
	}
	return b.String()
}

func terminalRowsMessage(title string, rows []terminalRow) string {
	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\n")
	for _, row := range rows {
		fmt.Fprintf(&b, "  %-6s  %s\n", row.Label, row.Value)
	}
	return b.String()
}

func writeTerminalOutput(w io.Writer, message string) {
	if !terminalInteractive(w) {
		fmt.Fprint(w, message)
		return
	}
	if terminalColorEnabled() {
		message = colorizeTerminalMessage(message)
	}
	for _, line := range strings.SplitAfter(message, "\n") {
		fmt.Fprint(w, line)
		time.Sleep(18 * time.Millisecond)
	}
}

func terminalInteractive(w io.Writer) bool {
	if os.Getenv("CI") != "" || os.Getenv("ARKROUTE_NO_EFFECTS") != "" {
		return false
	}
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func terminalColorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if runtime.GOOS == "windows" {
		return os.Getenv("WT_SESSION") != "" ||
			os.Getenv("ANSICON") != "" ||
			strings.Contains(strings.ToLower(os.Getenv("TERM")), "xterm")
	}
	return os.Getenv("TERM") != "dumb"
}

func colorizeTerminalMessage(message string) string {
	const (
		reset  = "\x1b[0m"
		dim    = "\x1b[2m"
		accent = "\x1b[38;5;120m"
		bright = "\x1b[97m"
	)
	var b strings.Builder
	for _, line := range strings.SplitAfter(message, "\n") {
		body := strings.TrimSuffix(line, "\n")
		newline := ""
		if strings.HasSuffix(line, "\n") {
			newline = "\n"
		}
		switch {
		case strings.HasPrefix(body, ">_ arkroute"):
			b.WriteString(accent + body + reset + newline)
		case body == "setup panel" || body == "control panel" || body == "gateway" || body == "provider":
			b.WriteString(bright + body + reset + newline)
		case strings.HasPrefix(body, "  ") && len(body) >= 10:
			b.WriteString(dim + body[:10] + reset + body[10:] + newline)
		default:
			b.WriteString(body + newline)
		}
	}
	return b.String()
}
