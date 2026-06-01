package app

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"bat.dev/arkrouter/internal/adapter/builtin"
	"bat.dev/arkrouter/internal/client/claude"
	"bat.dev/arkrouter/internal/config"
	"bat.dev/arkrouter/internal/observability"
	"bat.dev/arkrouter/internal/router"
	arkruntime "bat.dev/arkrouter/internal/runtime"
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
	executor := arkruntime.NewExecutor(arkruntime.Deps{
		Snapshot: snapshot,
		Router:   router.New(snapshot, health),
		Adapters: builtin.DefaultRegistry(),
		Health:   health,
		Trace:    observability.NewNoopSink(),
		Client:   &http.Client{Timeout: time.Duration(cfg.Server.UpstreamTimeoutSeconds) * time.Second},
	})
	server := claude.NewServer(claude.Deps{Snapshot: snapshot, Executor: executor})
	addr := net.JoinHostPort(cfg.Server.Host, strconv.Itoa(cfg.Server.Port))
	fmt.Printf("arkrouter listening on http://%s\n", addr)
	return http.ListenAndServe(addr, server.Routes())
}
