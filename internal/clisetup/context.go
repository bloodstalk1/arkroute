package clisetup

import (
	"errors"
	"fmt"
	"runtime"
	"strings"

	"github.com/bloodstalk1/arkroute/internal/config"
	"github.com/bloodstalk1/arkroute/internal/security"
	"github.com/bloodstalk1/arkroute/internal/strutil"
)

var (
	ErrSelectionRequired = errors.New("model_id or route_alias is required")
	ErrModelNotFound    = errors.New("model not found")
	ErrRouteNotFound    = errors.New("route not found")
)

type Request struct {
	ModelID    string
	RouteAlias string
}

type Context struct {
	SchemaVersion int       `json:"schema_version"`
	SelectionType string    `json:"selection_type"`
	ModelID       string    `json:"model_id,omitempty"`
	RouteAlias    string    `json:"route_alias,omitempty"`
	ProviderID    string    `json:"provider_id,omitempty"`
	UpstreamModel string    `json:"upstream_model,omitempty"`
	SelectedAlias string    `json:"selected_alias"`
	BaseURL       string    `json:"base_url"`
	OpenAIBaseURL string    `json:"openai_base_url"`
	Profiles      []Profile `json:"profiles"`
}

type Profile struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Protocol       string   `json:"protocol"`
	Command        string   `json:"command"`
	ModelAlias     string   `json:"model_alias"`
	ModelDiscovery bool     `json:"model_discovery"`
	LaunchSupported bool    `json:"launch_supported"`
	Notes          []string `json:"notes,omitempty"`
}

func BuildContext(cfg config.Config, req Request) (Context, error) {
	selection, err := resolveSelection(cfg, req)
	if err != nil {
		return Context{}, err
	}
	baseURL := config.LocalGatewayBaseURL(cfg)
	openAIBaseURL := strings.TrimRight(baseURL, "/") + "/v1"
	return Context{
		SchemaVersion: 1,
		SelectionType: selection.kind,
		ModelID:       selection.modelID,
		RouteAlias:    selection.routeAlias,
		ProviderID:    selection.providerID,
		UpstreamModel: selection.upstreamModel,
		SelectedAlias: selection.alias,
		BaseURL:       baseURL,
		OpenAIBaseURL: openAIBaseURL,
		Profiles: []Profile{
			claudeProfile(baseURL, selection.alias),
			openAIProfile("opencode", "OpenCode", openAIBaseURL, selection.alias),
			openAIProfile("codex", "Codex", openAIBaseURL, selection.alias),
			droidProfile(openAIBaseURL, selection.alias),
		},
	}, nil
}

type selection struct {
	kind          string
	modelID       string
	routeAlias    string
	providerID    string
	upstreamModel string
	alias         string
}

func resolveSelection(cfg config.Config, req Request) (selection, error) {
	if strings.TrimSpace(req.RouteAlias) != "" {
		for _, route := range cfg.Routes {
			if route.Alias == req.RouteAlias {
				return selection{kind: "route", routeAlias: route.Alias, alias: route.Alias}, nil
			}
		}
		return selection{}, fmt.Errorf("%w: %s", ErrRouteNotFound, req.RouteAlias)
	}
	if strings.TrimSpace(req.ModelID) != "" {
		for _, model := range cfg.Models {
			if model.ID == req.ModelID {
				alias := firstNonEmpty(model.ExposedAlias, model.ID)
				return selection{
					kind: "model", modelID: model.ID, providerID: model.ProviderID,
					upstreamModel: model.UpstreamModel, alias: alias,
				}, nil
			}
		}
		return selection{}, fmt.Errorf("%w: %s", ErrModelNotFound, req.ModelID)
	}
	return selection{}, ErrSelectionRequired
}

func claudeProfile(baseURL string, alias string) Profile {
	command := fmt.Sprintf("eval \"$(arkroute activate claude)\"\n# choose model alias in Claude Code: %s", shellQuote(alias))
	if runtime.GOOS == "windows" {
		command = fmt.Sprintf("arkroute activate claude | Invoke-Expression\nREM choose model alias in Claude Code: %s", alias)
	}
	return Profile{
		ID: "claude", Name: "Claude Code", Protocol: "anthropic",
		Command: command, ModelAlias: alias, ModelDiscovery: true, LaunchSupported: true,
		Notes: []string{"Uses ANTHROPIC_BASE_URL and gateway model discovery."},
	}
}

func openAIProfile(id string, name string, baseURL string, alias string) Profile {
	command := fmt.Sprintf("eval \"$(arkroute activate %s)\"\nexport OPENAI_MODEL=%s", id, shellQuote(alias))
	if runtime.GOOS == "windows" {
		command = fmt.Sprintf("arkroute activate %s | Invoke-Expression\nset OPENAI_MODEL=%s", id, shellQuote(alias))
	}
	return Profile{
		ID: id, Name: name, Protocol: "openai_compatible",
		Command: command, ModelAlias: alias, ModelDiscovery: false, LaunchSupported: false,
		Notes: []string{"Uses Arkroute's local /v1 endpoint and server.client_key."},
	}
}

func droidProfile(baseURL string, alias string) Profile {
	command := fmt.Sprintf("eval \"$(arkroute activate droid)\"\nexport ARKROUTE_OPENAI_MODEL=%s\n# droidrun run --provider OpenAILike --model \"$ARKROUTE_OPENAI_MODEL\" --api_base \"$ARKROUTE_OPENAI_BASE_URL\" \"Open the settings app\"", shellQuote(alias))
	if runtime.GOOS == "windows" {
		command = fmt.Sprintf("arkroute activate droid | Invoke-Expression\nset ARKROUTE_OPENAI_MODEL=%s\nREM droidrun run --provider OpenAILike --model \"%%ARKROUTE_OPENAI_MODEL%%\" --api_base \"%%ARKROUTE_OPENAI_BASE_URL%%\" \"Open the settings app\"", shellQuote(alias))
	}
	return Profile{
		ID: "droid", Name: "Droid / OpenAI-like", Protocol: "openai_compatible",
		Command: command, ModelAlias: alias, ModelDiscovery: false, LaunchSupported: false,
		Notes: []string{"DroidRun passes the OpenAI-like base URL as --api_base."},
	}
}

func shellQuote(value string) string {
	return security.ShellQuote(value)
}

func firstNonEmpty(values ...string) string {
	return strutil.FirstNonEmpty(values...)
}
