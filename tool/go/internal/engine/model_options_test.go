package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fake provider records exact argv entries and returns a bounded fixture result.
func modelOptionProvider(t *testing.T, provider string, repair bool) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "provider")
	script := `#!/bin/sh
printf '%s\n' CALL "$@" >> "$0.args"
cat >/dev/null
answer='{"answer":7}'
`
	if repair {
		script += `if [ ! -f "$0.called" ]; then
  touch "$0.called"
  answer='{}'
fi
`
	}
	if provider == "codex" {
		script += `out=''
while [ "$#" -gt 0 ]; do
  if [ "$1" = '--output-last-message' ]; then
    shift
    out="$1"
  fi
  shift
done
printf '%s\n' "$answer" > "$out"
printf '%s\n' '{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}'
`
	} else {
		script += `printf '{"type":"result","is_error":false,"result":"fixture","structured_output":%s}\n' "$answer"
`
	}
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return bin
}

func capturedModelCalls(t *testing.T, bin string) [][]string {
	t.Helper()
	raw, err := os.ReadFile(bin + ".args")
	if err != nil {
		t.Fatal(err)
	}
	var calls [][]string
	for _, call := range strings.Split(string(raw), "CALL\n")[1:] {
		calls = append(calls, strings.Split(strings.TrimSuffix(call, "\n"), "\n"))
	}
	return calls
}

func requireModelArgument(t *testing.T, args []string, flag, value string) {
	t.Helper()
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == value {
			return
		}
	}
	t.Fatalf("missing argv pair %q %q in %q", flag, value, args)
}

func requireSelectedModelAndEffort(t *testing.T, provider string, args []string, model, effort string) {
	t.Helper()
	if provider == "codex" {
		requireModelArgument(t, args, "-c", fmt.Sprintf("model=%q", model))
		requireModelArgument(t, args, "-c", fmt.Sprintf("model_reasoning_effort=%q", effort))
	} else {
		requireModelArgument(t, args, "--model", model)
		requireModelArgument(t, args, "--effort", effort)
	}
}

func TestCLIModelAndEffortReachEveryProviderPhase(t *testing.T) {
	for _, provider := range []string{"codex", "claude"} {
		for phase := range phaseEffort {
			t.Run(provider+"/"+phase, func(t *testing.T) {
				bin := modelOptionProvider(t, provider, false)
				model := `gateway/model $(literal) "quoted"`
				cfg := Config{Agents: map[string]AgentConfig{provider: {
					Bin: bin, Model: model, Effort: "high", EffortByPhase: map[string]string{phase: "medium"},
				}}, EffortOverrides: map[string]string{provider: "xhigh"}}
				result, err := CallProvider(context.Background(), provider, Request{Phase: phase, Mode: ModeFor(phase),
					CWD: t.TempDir(), RunDir: t.TempDir(), Body: "fixture", Effort: "low"}, cfg, nil)
				if err != nil || !result.OK {
					t.Fatal(result, err)
				}
				calls := capturedModelCalls(t, bin)
				if len(calls) != 1 {
					t.Fatal("unexpected number of provider calls", len(calls))
				}
				requireSelectedModelAndEffort(t, provider, calls[0], model, "xhigh")
			})
		}
	}
}

func TestCLIEffortOverridePreservesExistingPrecedenceWhenOmitted(t *testing.T) {
	for _, tc := range []struct {
		name, request, global, phase, want string
		overrides                          map[string]string
	}{
		{"request", "low", "high", "medium", "low", nil},
		{"phase", "", "high", "medium", "medium", nil},
		{"global", "", "high", "", "high", nil},
		{"default", "", "", "", "low", nil},
		{"other-provider", "low", "high", "medium", "low", map[string]string{"codex": "xhigh"}},
		{"opaque-cli", "low", "high", "medium", "future-effort", map[string]string{"claude": "future-effort"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin := modelOptionProvider(t, "claude", false)
			cfg := Config{Agents: map[string]AgentConfig{"claude": {Bin: bin, Model: "chosen-model", Effort: tc.global, EffortByPhase: map[string]string{"doctor": tc.phase}}},
				EffortOverrides: tc.overrides}
			result, err := CallProvider(context.Background(), "claude", Request{Phase: "doctor", CWD: t.TempDir(), RunDir: t.TempDir(), Effort: tc.request}, cfg, nil)
			if err != nil || !result.OK {
				t.Fatal(result, err)
			}
			requireSelectedModelAndEffort(t, "claude", capturedModelCalls(t, bin)[0], "chosen-model", tc.want)
		})
	}
}

func TestCLIModelAndEffortRemainSelectedDuringSchemaRepair(t *testing.T) {
	schema := map[string]any{"type": "object", "required": []any{"answer"}, "properties": map[string]any{"answer": map[string]any{"type": "integer"}}}
	for _, provider := range []string{"codex", "claude"} {
		t.Run(provider, func(t *testing.T) {
			bin := modelOptionProvider(t, provider, true)
			cfg := Config{Agents: map[string]AgentConfig{provider: {Bin: bin, Model: "chosen-model", Effort: "medium"}},
				EffortOverrides: map[string]string{provider: "xhigh"}}
			result, err := CallWithRepair(context.Background(), provider, Request{Phase: "execute", Mode: "write", CWD: t.TempDir(),
				RunDir: t.TempDir(), Body: "fixture", Schema: schema, Effort: "low"}, cfg, nil)
			if err != nil || !result.OK || result.Processes != 2 {
				t.Fatal(result, err)
			}
			calls := capturedModelCalls(t, bin)
			if len(calls) != 2 {
				t.Fatal("schema repair did not make exactly two calls", len(calls))
			}
			for _, args := range calls {
				requireSelectedModelAndEffort(t, provider, args, "chosen-model", "xhigh")
			}
			if provider == "codex" {
				requireModelArgument(t, calls[0], "--sandbox", "workspace-write")
				requireModelArgument(t, calls[1], "--sandbox", "read-only")
			} else {
				requireModelArgument(t, calls[0], "--permission-mode", "acceptEdits")
				requireModelArgument(t, calls[1], "--permission-mode", "plan")
			}
		})
	}
}
