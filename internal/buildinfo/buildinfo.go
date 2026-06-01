package buildinfo

import (
	"fmt"
	"runtime"
)

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func Summary() string {
	return "arkrouter " + Version
}

func Debug() string {
	return fmt.Sprintf("version: %s\ncommit: %s\nbuild_date: %s\ngo: %s\nos_arch: %s/%s\n", Version, Commit, BuildDate, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
