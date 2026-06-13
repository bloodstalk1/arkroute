package clitools

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/security"
)

const schemaVersion = 1

type ToolStatus struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Command             string `json:"command"`
	Installed           bool   `json:"installed"`
	GatewayReachable    bool   `json:"gateway_reachable"`
	BaseURL             string `json:"base_url"`
	ModelDiscovery      bool   `json:"model_discovery"`
	LaunchSupported     bool   `json:"launch_supported"`
	LaunchBlockedReason string `json:"launch_blocked_reason"`
	ActivationCommand   string `json:"activation_command"`
}

type ListResponse struct {
	SchemaVersion int          `json:"schema_version"`
	Tools         []ToolStatus `json:"tools"`
}

type LaunchResponse struct {
	SchemaVersion int    `json:"schema_version"`
	Launched      bool   `json:"launched"`
	PID           int    `json:"pid"`
	Command       string `json:"command"`
}

type ProcessSpec struct {
	Command string
	Env     []string
}

type Service struct {
	ConfigPath               string
	GatewayHosted            bool
	LookupPath               func(string) (string, error)
	GatewayReachable         func(config.Config) bool
	HasInteractiveTerminal   func() bool
	StartProcess             func(ProcessSpec) (int, error)
	Environ                  func() []string
	ActivationCommandBuilder func(config.Config) string
	OnProcessExit            func(command string, pid int, exitErr error)
}

type Error struct {
	Code        string `json:"code"`
	Message     string `json:"error"`
	Remediation string `json:"remediation"`
	Status      int    `json:"-"`
}

func (e *Error) Error() string {
	return e.Message
}

func NewService(configPath string, gatewayHosted bool) *Service {
	return &Service{ConfigPath: configPath, GatewayHosted: gatewayHosted}
}

func (s *Service) Status() (ListResponse, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return ListResponse{}, err
	}
	_, lookupErr := s.lookupPath()("claude")
	installed := lookupErr == nil
	gatewayReachable := s.gatewayReachable()(cfg)
	launchSupported, blocked := s.launchSupport(installed, gatewayReachable)
	return ListResponse{
		SchemaVersion: schemaVersion,
		Tools: []ToolStatus{{
			ID:                  "claude",
			Name:                "Claude Code",
			Command:             "claude",
			Installed:           installed,
			GatewayReachable:    gatewayReachable,
			BaseURL:             BaseURL(cfg),
			ModelDiscovery:      true,
			LaunchSupported:     launchSupported,
			LaunchBlockedReason: blocked,
			ActivationCommand:   s.activationCommand()(cfg),
		}},
	}, nil
}

func (s *Service) LaunchClaude() (LaunchResponse, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return LaunchResponse{}, err
	}
	path, err := s.lookupPath()("claude")
	if err != nil || path == "" {
		return LaunchResponse{}, &Error{Code: "missing_binary", Message: "Claude Code binary not found", Remediation: "Install Claude Code, then reopen CLI Tools.", Status: http.StatusBadRequest}
	}
	if !s.gatewayReachable()(cfg) {
		return LaunchResponse{}, &Error{Code: "gateway_unreachable", Message: "Arkroute gateway is not reachable", Remediation: "Start arkroute serve, then retry.", Status: http.StatusBadRequest}
	}
	launchSupported, _ := s.launchSupport(true, true)
	if !launchSupported {
		return LaunchResponse{}, &Error{Code: "launch_unavailable", Message: "Interactive launch is unavailable", Remediation: "Copy the activation command and run Claude Code in a terminal.", Status: http.StatusBadRequest}
	}
	pid, err := s.startProcess()(ProcessSpec{Command: path, Env: ClaudeLaunchEnv(s.environ(), cfg)})
	if err != nil {
		return LaunchResponse{}, &Error{Code: "spawn_failed", Message: "Failed to launch Claude Code", Remediation: "Copy the activation command and run Claude Code in a terminal.", Status: http.StatusInternalServerError}
	}
	return LaunchResponse{SchemaVersion: schemaVersion, Launched: true, PID: pid, Command: "claude"}, nil
}

func (s *Service) loadConfig() (config.Config, error) {
	path := s.ConfigPath
	if path == "" {
		path = defaultConfigPath()
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		return config.Config{}, &Error{Code: "config_error", Message: err.Error(), Remediation: "Run arkroute setup.", Status: http.StatusBadRequest}
	}
	if err := validateConfig(cfg); err != nil {
		return config.Config{}, &Error{Code: "config_error", Message: err.Error(), Remediation: "Run arkroute setup.", Status: http.StatusBadRequest}
	}
	return cfg, nil
}

func (s *Service) lookupPath() func(string) (string, error) {
	if s.LookupPath != nil {
		return s.LookupPath
	}
	return exec.LookPath
}

func (s *Service) gatewayReachable() func(config.Config) bool {
	if s.GatewayReachable != nil {
		return s.GatewayReachable
	}
	return DefaultGatewayReachable
}

func (s *Service) hasInteractiveTerminal() bool {
	if s.HasInteractiveTerminal != nil {
		return s.HasInteractiveTerminal()
	}
	return DefaultHasInteractiveTerminal()
}

func (s *Service) startProcess() func(ProcessSpec) (int, error) {
	if s.StartProcess != nil {
		return s.StartProcess
	}
	return func(spec ProcessSpec) (int, error) {
		return DefaultStartProcessWithExit(spec, s.OnProcessExit)
	}
}

func (s *Service) environ() []string {
	if s.Environ != nil {
		return s.Environ()
	}
	return os.Environ()
}

func (s *Service) activationCommand() func(config.Config) string {
	if s.ActivationCommandBuilder != nil {
		return s.ActivationCommandBuilder
	}
	return ActivationCommand
}

func (s *Service) launchSupport(installed bool, gatewayReachable bool) (bool, string) {
	if !installed {
		return false, "claude binary not found"
	}
	if !gatewayReachable {
		return false, "gateway unreachable"
	}
	if !s.GatewayHosted {
		return false, "panel is not hosted by the running gateway"
	}
	if !s.hasInteractiveTerminal() {
		return false, "interactive terminal unavailable"
	}
	return true, ""
}

func ClaudeLaunchEnv(base []string, cfg config.Config) []string {
	env := make([]string, 0, len(base)+4)
	for _, item := range base {
		key, _, _ := strings.Cut(item, "=")
		if strings.HasPrefix(key, "ANTHROPIC_") {
			continue
		}
		env = append(env, item)
	}
	env = append(env,
		"ANTHROPIC_BASE_URL="+BaseURL(cfg),
		"ANTHROPIC_AUTH_TOKEN="+cfg.Server.ClientKey,
		"CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=1",
		"CLAUDE_CODE_AUTO_COMPACT_WINDOW=190000",
	)
	return env
}

func BaseURL(cfg config.Config) string {
	host := strings.TrimSpace(cfg.Server.Host)
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return fmt.Sprintf("http://%s:%d", host, cfg.Server.Port)
}

func ActivationCommand(config.Config) string {
	if runtime.GOOS == "windows" {
		return "arkroute activate claude | Invoke-Expression"
	}
	return `eval "$(arkroute activate claude)"`
}

func DefaultGatewayReachable(cfg config.Config) bool {
	client := http.Client{Timeout: 750 * time.Millisecond}
	resp, err := client.Get(BaseURL(cfg) + "/healthz")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func DefaultHasInteractiveTerminal() bool {
	for _, file := range []*os.File{os.Stdin, os.Stdout, os.Stderr} {
		info, err := file.Stat()
		if err != nil || info.Mode()&os.ModeCharDevice == 0 {
			return false
		}
	}
	return true
}

func DefaultStartProcess(spec ProcessSpec) (int, error) {
	return DefaultStartProcessWithExit(spec, nil)
}

func DefaultStartProcessWithExit(spec ProcessSpec, onExit func(command string, pid int, exitErr error)) (int, error) {
	cmd := exec.Command(spec.Command)
	cmd.Env = spec.Env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	go func() {
		waitErr := cmd.Wait()
		if onExit == nil {
			return
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer func() {
				if r := recover(); r != nil {
					fmt.Fprintf(os.Stderr, "arkroute: OnProcessExit panic for %s (pid %d): %v\n", spec.Command, pid, r)
				}
			}()
			onExit(spec.Command, pid, waitErr)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			fmt.Fprintf(os.Stderr, "arkroute: OnProcessExit for %s (pid %d) exceeded 5s budget\n", spec.Command, pid)
		}
	}()
	return pid, nil
}

func validateConfig(cfg config.Config) error {
	if strings.TrimSpace(cfg.Server.ClientKey) == "" {
		return fmt.Errorf("server.client_key is required")
	}
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}
	if !security.IsLoopbackHost(cfg.Server.Host) {
		return fmt.Errorf("server.host must be loopback for CLI tool launch")
	}
	return nil
}

func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".arkroute/config.yaml"
	}
	return filepath.Join(home, ".arkroute", "config.yaml")
}
