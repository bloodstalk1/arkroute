package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/bloodstalk1/arkroute/internal/config"
)

type UninstallOptions struct {
	ConfigPath   string
	SettingsPath string
	Purge        bool
	Yes          bool
}

func Uninstall(options UninstallOptions, w io.Writer) error {
	path := pathOrDefault(options.ConfigPath)
	cfg, err := config.LoadFile(path)
	if err != nil {
		return err
	}
	removed, err := RemoveClaudeSettings(options.SettingsPath, cfg)
	if err != nil {
		return err
	}
	if removed {
		fmt.Fprintf(w, "Claude settings integration removed: %s\n", ClaudeSettingsPath(options.SettingsPath))
	} else {
		fmt.Fprintln(w, "Claude settings were not changed; current values do not point at Arkroute")
	}
	if options.Purge {
		if !options.Yes {
			return fmt.Errorf("purge requires --yes for non-interactive use")
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		logPath := DefaultLogPath()
		if err := os.Remove(logPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		removeDefaultArkrouteDirIfEmpty(path)
		fmt.Fprintln(w, "Arkroute config and logs deleted")
		return nil
	}
	fmt.Fprintf(w, "\nTo remove the npm-installed binary:\n  npm uninstall -g arkroute\n\nLocal config kept:\n  %s\n", path)
	return nil
}

func removeDefaultArkrouteDirIfEmpty(configPath string) {
	defaultPath := DefaultConfigPath()
	if filepath.Clean(configPath) != filepath.Clean(defaultPath) {
		return
	}
	_ = os.Remove(filepath.Dir(defaultPath))
}
