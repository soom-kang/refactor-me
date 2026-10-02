package controller

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/soom-kang/refactor-me/tool/go/internal/surface"
)

func TestRequestedSettingsReachDoctorAndRunEngine(t *testing.T) {
	for _, order := range [][]string{{"codex", "claude"}, {"claude", "codex"}} {
		cfg := surface.DefaultConfig()
		c := surface.Context{Config: cfg, Providers: order, ProviderSettings: map[string]surface.ProviderSettings{
			order[0]: {RequestedModel: "primary-model", CLIEffort: "xhigh"},
			order[1]: {RequestedModel: "fallback-model", CLIEffort: "high"},
		}}
		config := engineConfigForContext(c)
		if config.EffortOverrides[order[0]] != "xhigh" || config.EffortOverrides[order[1]] != "high" {
			t.Fatal("CLI effort lost before provider call", config.EffortOverrides)
		}
		settings := providerSettings(c)
		report := doctorReport{ProviderSettings: settings}
		text := renderDoctor(report)
		if !strings.Contains(text, "primary-model") || !strings.Contains(text, "CLI effort=xhigh") || !strings.Contains(text, "model access requires a live check") {
			t.Fatal("doctor settings missing or presented as model access evidence", text)
		}
		encoded, err := json.Marshal(report)
		if err != nil || !strings.Contains(string(encoded), `"providerSettings"`) {
			t.Fatal(string(encoded), err)
		}
	}
}

func TestOfflineDoctorSettingsRetainBlankModelAndOmitCLIOverride(t *testing.T) {
	c := surface.Context{Config: surface.DefaultConfig(), Providers: []string{"codex"}}
	settings := providerSettings(c)
	if len(settings) != 1 || settings["codex"].RequestedModel != "" {
		t.Fatal(settings)
	}
	encoded, err := json.Marshal(settings)
	if err != nil || strings.Contains(string(encoded), "cliEffort") || !strings.Contains(string(encoded), `"requestedModel":""`) {
		t.Fatal(string(encoded), err)
	}
	if config := engineConfigForContext(c); len(config.EffortOverrides) != 0 {
		t.Fatal("offline defaults gained an effort override", config.EffortOverrides)
	}
}
