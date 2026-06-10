package clisetup

import (
	"runtime"
	"strings"
	"testing"

	"github.com/bloodstalk1/arkroute/internal/config"
)

func TestContextForModelReturnsAllCLIProfiles(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	ctx, err := BuildContext(cfg, Request{ModelID: cfg.Models[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	if ctx.SchemaVersion != 1 {
		t.Fatalf("schema = %d, want 1", ctx.SchemaVersion)
	}
	if ctx.SelectedAlias != cfg.Models[0].ExposedAlias {
		t.Fatalf("alias = %q, want %q", ctx.SelectedAlias, cfg.Models[0].ExposedAlias)
	}
	wantProfiles := []string{"claude", "opencode", "codex", "droid"}
	for _, id := range wantProfiles {
		profile, ok := findProfile(ctx.Profiles, id)
		if !ok {
			t.Fatalf("profile %q missing from %+v", id, ctx.Profiles)
		}
		if strings.Contains(profile.Command, "sk-secret") {
			t.Fatalf("profile %s leaked upstream secret: %+v", id, profile)
		}
	}
}

func TestContextForRouteUsesRouteAlias(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	ctx, err := BuildContext(cfg, Request{RouteAlias: "sonnet"})
	if err != nil {
		t.Fatal(err)
	}
	if ctx.SelectedAlias != "sonnet" {
		t.Fatalf("alias = %q, want sonnet", ctx.SelectedAlias)
	}
	openCode, ok := findProfile(ctx.Profiles, "opencode")
	if !ok {
		t.Fatal("opencode profile missing")
	}
	wantSubstr := "export OPENAI_MODEL='sonnet'"
	if runtime.GOOS == "windows" {
		wantSubstr = `set OPENAI_MODEL="sonnet"`
	}
	if !strings.Contains(openCode.Command, wantSubstr) {
		t.Fatalf("command = %q, want substring %q", openCode.Command, wantSubstr)
	}
}

func TestContextRejectsUnknownSelection(t *testing.T) {
	cfg := config.MinimalValidConfig("local-key")
	if _, err := BuildContext(cfg, Request{ModelID: "missing"}); err == nil {
		t.Fatal("BuildContext error = nil, want missing model error")
	}
	if _, err := BuildContext(cfg, Request{RouteAlias: "missing"}); err == nil {
		t.Fatal("BuildContext route error = nil, want missing route error")
	}
}

func findProfile(profiles []Profile, id string) (Profile, bool) {
	for _, profile := range profiles {
		if profile.ID == id {
			return profile, true
		}
	}
	return Profile{}, false
}
