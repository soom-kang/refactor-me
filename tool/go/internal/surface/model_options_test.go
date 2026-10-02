package surface

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestProviderOptionsMapRolesAndPreserveConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, primary, fallback string
	}{
		{"codex-first", "codex", "claude"},
		{"claude-first", "claude", "codex"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			before, _ := json.Marshal(cfg)
			args := Args{Command: "run", Provider: tc.primary, Fallback: tc.fallback, FallbackSet: true,
				Model: "primary-model", FallbackModel: "fallback-model", Effort: "custom-effort", FallbackEffort: "xhigh"}
			order, err := ProviderOrder(cfg, args)
			if err != nil {
				t.Fatal(err)
			}
			resolved, settings, err := providerOptions(cfg, args, order)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]ProviderSettings{
				tc.primary:  {RequestedModel: "primary-model", CLIEffort: "custom-effort"},
				tc.fallback: {RequestedModel: "fallback-model", CLIEffort: "xhigh"},
			}
			if !reflect.DeepEqual(settings, want) {
				t.Fatalf("settings=%+v want=%+v", settings, want)
			}
			for provider, setting := range want {
				if ConfigString(resolved, "agents", provider, "model") != setting.RequestedModel {
					t.Fatalf("model for %s not overridden", provider)
				}
				if ConfigString(resolved, "agents", provider, "effort") != "" {
					t.Fatal("CLI effort changed project effort settings")
				}
			}
			after, _ := json.Marshal(cfg)
			if !bytes.Equal(before, after) {
				t.Fatal("configuration input mutated")
			}
		})
	}
}

func TestExecuteProviderOptionsPreserveProjectFile(t *testing.T) {
	for _, command := range []string{"run", "doctor"} {
		t.Run(command, func(t *testing.T) {
			repo := testRepo(t)
			if err := os.MkdirAll(filepath.Join(repo, ".refactor"), 0700); err != nil {
				t.Fatal(err)
			}
			configPath := filepath.Join(repo, ".refactor", "config.json")
			before := []byte(`{"schema_version":2,"agents":{"primary":"claude","fallback":"codex","claude":{"model":"configured-primary","effort":"low","effort_by_phase":{"audit":"medium"}},"codex":{"model":"configured-fallback"}},"future":{"keep":true}}` + "\n")
			if err := os.WriteFile(configPath, before, 0600); err != nil {
				t.Fatal(err)
			}
			called := false
			check := func(c Context) {
				called = true
				if !reflect.DeepEqual(c.Providers, []string{"claude", "codex"}) {
					t.Fatalf("provider order=%v", c.Providers)
				}
				want := map[string]ProviderSettings{"claude": {RequestedModel: "runtime-primary", CLIEffort: "xhigh"}, "codex": {RequestedModel: "configured-fallback"}}
				if !reflect.DeepEqual(c.ProviderSettings, want) {
					t.Fatalf("settings=%+v", c.ProviderSettings)
				}
				if ConfigString(c.Config, "agents", "claude", "model") != "runtime-primary" || ConfigString(c.Config, "agents", "claude", "effort") != "low" ||
					ConfigString(c.Config, "agents", "claude", "effort_by_phase", "audit") != "medium" || obj(c.Config["future"])["keep"] != true {
					t.Fatal("runtime model did not preserve other settings")
				}
			}
			callbacks := Callbacks{
				Run: func(c Context) (RunResult, error) {
					check(c)
					return RunResult{Status: "NO_CHANGES", RunID: "test", RunDir: t.TempDir()}, nil
				},
				Doctor: func(c Context) (DoctorResult, error) { check(c); return DoctorResult{OK: true}, nil },
			}
			var stdout, stderr bytes.Buffer
			if code := Execute([]string{command, "--model", "runtime-primary", "--effort", "xhigh"}, repo, &stdout, &stderr, callbacks); code != ExitOK || !called {
				t.Fatalf("exit=%d called=%v error=%s", code, called, stderr.String())
			}
			after, err := os.ReadFile(configPath)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("project configuration file changed", err)
			}
		})
	}
}

func TestExecuteRequiresModelsBeforeCallbacks(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"run", []string{"run", "--fallback", "none"}},
		{"run-without-live-doctor", []string{"run", "--fallback", "none", "--no-live-probe"}},
		{"live-doctor", []string{"doctor", "--fallback", "none"}},
		{"missing-fallback", []string{"run", "--model", "primary-model"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := testRepo(t)
			callbacks := Callbacks{
				Run:    func(Context) (RunResult, error) { t.Fatal("run callback started"); return RunResult{}, nil },
				Doctor: func(Context) (DoctorResult, error) { t.Fatal("doctor callback started"); return DoctorResult{}, nil },
			}
			var stdout, stderr bytes.Buffer
			if code := Execute(tc.args, repo, &stdout, &stderr, callbacks); code != ExitAborted || !strings.Contains(stderr.String(), "model for ") {
				t.Fatalf("exit=%d error=%s", code, stderr.String())
			}
			if _, err := os.Stat(filepath.Join(repo, ".refactor")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("model guard created project state", err)
			}
		})
	}
}

func TestProviderOptionsExistingModelAndBlankCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name, configured, override string
		live, wantError            bool
	}{
		{"configured-model", "custom-model", "", true, false},
		{"explicit-blank", "", "", true, true},
		{"blank-overridden", "", "chosen-model", true, false},
		{"whitespace-config", "  ", "", true, true},
		{"offline-blank", "", "", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			obj(obj(cfg["agents"])["codex"])["model"] = tc.configured
			resolved, settings, err := providerOptions(cfg, Args{Command: "doctor", Live: tc.live, Model: tc.override}, []string{"codex"})
			if (err != nil) != tc.wantError {
				t.Fatalf("error=%v wantError=%v", err, tc.wantError)
			}
			if err == nil {
				want := tc.configured
				if tc.override != "" {
					want = tc.override
				}
				if ConfigString(resolved, "agents", "codex", "model") != want || settings["codex"].RequestedModel != want {
					t.Fatal("resolved model changed")
				}
			}
		})
	}
}

func TestOfflineDoctorAllowsMissingModels(t *testing.T) {
	repo := testRepo(t)
	called := false
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"doctor", "--no-live-probe"}, repo, &stdout, &stderr, Callbacks{Doctor: func(c Context) (DoctorResult, error) {
		called = true
		if c.Args.Live || c.ProviderSettings["codex"].RequestedModel != "" || c.ProviderSettings["claude"].RequestedModel != "" {
			t.Fatal("offline doctor settings changed")
		}
		return DoctorResult{OK: true}, nil
	}})
	if code != ExitOK || !called {
		t.Fatalf("exit=%d called=%v error=%s", code, called, stderr.String())
	}
}

func TestProviderOptionParseErrorsAndOpaqueValues(t *testing.T) {
	for _, flag := range []string{"--model", "--fallback-model", "--effort", "--fallback-effort"} {
		for _, value := range [][]string{nil, {""}, {"  "}, {"--json"}} {
			args := append([]string{"run", flag}, value...)
			if _, err := Parse(args); err == nil {
				t.Fatalf("accepted missing/blank value: %v", args)
			}
		}
		for _, command := range []string{"init", "report", "clean", "version"} {
			if _, err := Parse([]string{command, flag, "opaque-value"}); err == nil {
				t.Fatalf("accepted %s for %s", flag, command)
			}
		}
		if _, err := Parse([]string{"run", "--version", flag, "value"}); err == nil {
			t.Fatal("version accepted model/effort option")
		}
	}
	model, effort := `gateway/model $(literal) "quoted"`, "future-effort"
	args, err := Parse([]string{"run", "--model", model, "--fallback-model", model, "--effort", effort, "--fallback-effort", effort})
	if err != nil || args.Model != model || args.FallbackModel != model || args.Effort != effort || args.FallbackEffort != effort {
		t.Fatal("opaque option value changed", args, err)
	}
}

func TestFallbackOptionsRequireDistinctProvider(t *testing.T) {
	for _, flag := range []string{"--fallback-model", "--fallback-effort"} {
		for _, fallback := range []string{"none", "codex"} {
			repo := testRepo(t)
			var stdout, stderr bytes.Buffer
			code := Execute([]string{"doctor", "--no-live-probe", "--provider", "codex", "--fallback", fallback, flag, "value"}, repo, &stdout, &stderr, Callbacks{Doctor: func(Context) (DoctorResult, error) {
				t.Fatal("callback started with unused fallback option")
				return DoctorResult{}, nil
			}})
			if code != ExitAborted || !strings.Contains(stderr.String(), "distinct fallback provider") {
				t.Fatalf("exit=%d error=%s", code, stderr.String())
			}
		}
	}
}
