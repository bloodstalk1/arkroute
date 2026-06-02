package app

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"bat.dev/arkroute/internal/adapter/builtin"
	"bat.dev/arkroute/internal/client/claude"
	"bat.dev/arkroute/internal/config"
	"bat.dev/arkroute/internal/observability"
	"bat.dev/arkroute/internal/router"
	arkruntime "bat.dev/arkroute/internal/runtime"
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
	logPath := DefaultLogPath()
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return fmt.Errorf("create log directory: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open trace log: %w", err)
	}
	defer logFile.Close()
	trace := observability.NewJSONLSink(logFile)
	health := router.NewHealthStore()
	executor := arkruntime.NewExecutor(arkruntime.Deps{
		Snapshot: snapshot,
		Router:   router.New(snapshot, health),
		Adapters: builtin.DefaultRegistry(),
		Health:   health,
		Trace:    trace,
		Client:   &http.Client{Timeout: time.Duration(cfg.Server.UpstreamTimeoutSeconds) * time.Second},
	})
	server := claude.NewServer(claude.Deps{Snapshot: snapshot, Executor: executor})
	addr := net.JoinHostPort(cfg.Server.Host, strconv.Itoa(cfg.Server.Port))
	srv := &http.Server{Addr: addr, Handler: server.Routes()}
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()
	fmt.Printf("arkroute listening on http://%s\n", addr)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("server startup failed: %w", err)
		}
		return nil
	case <-stop:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			return fmt.Errorf("server shutdown failed: %w", err)
		}
		return nil
	}
}
