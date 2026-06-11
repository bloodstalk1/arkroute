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
	"time"

	"github.com/bloodstalk1/arkroute/internal/adapter/builtin"
	"github.com/bloodstalk1/arkroute/internal/client/claude"
	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/observability"
	"github.com/bloodstalk1/arkroute/internal/router"
	"github.com/bloodstalk1/arkroute/internal/security/ratelimit"
	arkruntime "github.com/bloodstalk1/arkroute/internal/runtime"
)

func Serve(path string) error {
	if path == "" {
		path = DefaultConfigPath()
	}
	cfg, err := config.LoadFile(path)
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
	state, err := arkruntime.NewState(arkruntime.StateDeps{
		ConfigPath:   path,
		ListenerHost: cfg.Server.Host,
		ListenerPort: cfg.Server.Port,
		Adapters:     builtin.DefaultRegistry(),
		Health:       health,
		Trace:        trace,
		NewHTTPClient: func(cfg config.Config) *http.Client {
			return &http.Client{Timeout: time.Duration(cfg.Server.UpstreamTimeoutSeconds) * time.Second}
		},
	})
	if err != nil {
		return err
	}
	server := claude.NewServer(claude.Deps{
		State:      state,
		ConfigPath: path,
		ClaudeSettingsWriter: func(cfg config.Config) error {
			return WriteClaudeSettings("", cfg)
		},
		RateLimiter: newRateLimiter(cfg),
	})
	addr := net.JoinHostPort(cfg.Server.Host, strconv.Itoa(cfg.Server.Port))
	srv := &http.Server{
		Addr:              addr,
		Handler:           server.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()
	message := serveReadyMessage(addr, path, logPath)
	if guidance := ServeSetupGuidance(cfg); guidance != "" {
		message += "\n" + guidance
	}
	writeTerminalOutput(os.Stdout, message)
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, shutdownSignals()...)
	defer signal.Stop(shutdown)

	reload := make(chan os.Signal, 1)
	if signals := reloadSignals(); len(signals) > 0 {
		signal.Notify(reload, signals...)
		defer signal.Stop(reload)
	}

	for {
		select {
		case err := <-errCh:
			if err != nil && err != http.ErrServerClosed {
				return fmt.Errorf("server startup failed: %w", err)
			}
			return nil
		case <-reload:
			result := state.Reload(context.Background(), arkruntime.ReloadSourceSignal, "signal_sighup")
			if result.Success {
				fmt.Printf("arkroute reloaded generation %d\n", result.Generation)
			} else {
				fmt.Fprintf(os.Stderr, "arkroute reload failed: %s\n", result.Error)
			}
		case <-shutdown:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := srv.Shutdown(ctx); err != nil {
				return fmt.Errorf("server shutdown failed: %w", err)
			}
			return nil
		}
	}
}

func ServeSetupGuidance(cfg config.Config) string {
	if HasUsableProvider(cfg) {
		return ""
	}
	return providerSetupGuidanceMessage()
}

func HasUsableProvider(cfg config.Config) bool {
	for _, provider := range cfg.Providers {
		if provider.Enabled {
			return true
		}
	}
	return false
}

func newRateLimiter(cfg config.Config) *ratelimit.Store {
	if cfg.Server.RateLimitRPM <= 0 {
		return nil
	}
	return ratelimit.New(time.Minute, cfg.Server.RateLimitRPM, 5)
}
