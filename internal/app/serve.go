package app

import (
	"fmt"
	"net"
	"net/http"
	"strconv"

	"bat.dev/arkrouter/internal/client/claude"
	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/router"
)

func Serve(path string) error {
	if path == "" {
		path = DefaultConfigPath()
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		return err
	}
	snapshot, err := config.BuildSnapshot(cfg)
	if err != nil {
		return err
	}
	health := router.NewHealthStore()
	server := claude.NewServer(claude.Deps{Snapshot: snapshot, Router: router.New(snapshot, health), Health: health})
	addr := net.JoinHostPort(cfg.Server.Host, strconv.Itoa(cfg.Server.Port))
	fmt.Printf("arkrouter listening on http://%s\n", addr)
	return http.ListenAndServe(addr, server.Routes())
}
