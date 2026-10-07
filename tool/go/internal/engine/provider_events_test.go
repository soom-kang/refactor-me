package engine

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type providerEventRecorder struct {
	events []ProviderEvent
}

func (r *providerEventRecorder) OnProviderEvent(event ProviderEvent) {
	r.events = append(r.events, event)
}

func TestProviderEventsRetainToolAccountingAndRawTranscripts(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "fake-provider")
			raw := ""
			script := "#!/bin/sh\ncat >/dev/null\n"
			var wantEvents []ProviderEvent
			if provider == "claude" {
				// Two tool_use parts in a single assistant event have always counted
				// as one recognized event, rather than two individual tool calls.
				raw = `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"RAW_PATH_FIXTURE"}},{"type":"tool_use","name":"Grep","input":{"pattern":"RAW_PATTERN_FIXTURE"}}]}}
{"type":"assistant","message":{"content":[{"type":"text","text":"RAW_TEXT_FIXTURE"}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"CustomTool","input":{"command":"RAW_COMMAND_FIXTURE"}}]}}
{"type":"result","is_error":false,"result":"RAW_DETAIL_FIXTURE","structured_output":{"answer":7},"permission_denials":[{"detail":"RAW_DENIAL_FIXTURE"},{"detail":"RAW_DENIAL_FIXTURE"}]}
`
				wantEvents = []ProviderEvent{{Kind: "permission_denied", Provider: provider, Phase: "audit", Count: 2}}
			} else {
				raw = `{"type":"item.started","item":{"type":"command_execution","command":"RAW_COMMAND_FIXTURE"}}
{"type":"item.completed","item":{"type":"command_execution","command":"RAW_COMMAND_FIXTURE"}}
{"type":"item.completed","item":{"type":"file_change","changes":[{"path":"RAW_PATH_FIXTURE"}]}}
{"type":"item.completed","item":{"type":"agent_message","text":"RAW_TEXT_FIXTURE"}}
{"type":"turn.completed","usage":{"input_tokens":3,"output_tokens":2}}
`
				script += `out=''
while [ "$#" -gt 0 ]; do
  if [ "$1" = '--output-last-message' ]; then
    shift
    out="$1"
  fi
  shift
done
printf '%s\n' '{"answer":7}' > "$out"
`
			}
			script += "cat <<'PROVIDER_JSONL'\n" + raw + "PROVIDER_JSONL\n"
			if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			log := &providerEventRecorder{}
			result, err := CallProvider(context.Background(), provider, Request{Phase: "audit", CWD: dir, RunDir: dir, Body: "prompt fixture"}, Config{Agents: map[string]AgentConfig{provider: {Bin: bin}}}, log)
			if err != nil || !result.OK || result.ToolCalls != 2 {
				t.Fatalf("provider=%+v, err=%v", result, err)
			}
			var transitions []ProviderEvent
			activityCount := 0
			for _, event := range log.events {
				if event.Kind != "activity" {
					transitions = append(transitions, event)
					continue
				}
				activityCount++
				if strings.Contains(event.Path, "RAW_") || strings.Contains(event.Action, "RAW_") {
					t.Fatal("activity exposed unverified path or command", event)
				}
			}
			if !reflect.DeepEqual(transitions, wantEvents) || activityCount == 0 {
				t.Fatalf("events=%+v, want transitions=%+v and activity", log.events, wantEvents)
			}
			completed := log.events[len(log.events)-1]
			if completed.Action != "provider" || completed.Status != "completed" || completed.DurationMS == nil || *completed.DurationMS != result.DurationMS ||
				completed.ToolCalls == nil || *completed.ToolCalls != result.ToolCalls || completed.ExitCode == nil || *completed.ExitCode != *result.ExitCode {
				t.Fatal("provider completion lost process metadata", completed, result)
			}
			transcript, err := os.ReadFile(result.RawPath)
			if err != nil || string(transcript) != raw {
				t.Fatalf("raw transcript changed: %q, %v", transcript, err)
			}
			if strings.Contains(result.RawPath, "RAW_") {
				t.Fatal("provider content changed the transcript path")
			}
		})
	}
}

func TestSchemaRepairEventContainsMetadataAndStaysReadOnly(t *testing.T) {
	schema := map[string]any{"type": "object", "required": []any{"answer"}, "properties": map[string]any{"answer": map[string]any{"type": "integer"}}}
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			bin := modelOptionProvider(t, provider, true)
			log := &providerEventRecorder{}
			runDir := t.TempDir()
			result, err := CallWithRepair(context.Background(), provider, Request{Phase: "execute", Mode: "write", CWD: t.TempDir(), RunDir: runDir, Body: "fixture", Schema: schema}, Config{Agents: map[string]AgentConfig{provider: {Bin: bin}}}, log)
			if err != nil || !result.OK || result.Processes != 2 {
				t.Fatal(result, err)
			}
			want := []ProviderEvent{{Kind: "schema_repair", Provider: provider, Phase: "execute", Count: 1}}
			var transitions []ProviderEvent
			for _, event := range log.events {
				if event.Kind != "activity" {
					transitions = append(transitions, event)
				}
			}
			if !reflect.DeepEqual(transitions, want) {
				t.Fatalf("events=%+v", log.events)
			}
			calls := capturedModelCalls(t, bin)
			if len(calls) != 2 {
				t.Fatal("repair call count changed", len(calls))
			}
			if provider == "codex" {
				requireModelArgument(t, calls[0], "--sandbox", "workspace-write")
				requireModelArgument(t, calls[1], "--sandbox", "read-only")
			} else {
				requireModelArgument(t, calls[0], "--permission-mode", "acceptEdits")
				requireModelArgument(t, calls[1], "--permission-mode", "plan")
			}
			for _, attempt := range []string{"primary", "repair"} {
				path := filepath.Join(runDir, "provider", "execute-"+provider+"-"+attempt+".stdout.jsonl")
				if _, err := os.Stat(path); err != nil {
					t.Fatal("missing transcript", path, err)
				}
			}
		})
	}
}

func TestActivityEventsVerifyPathsAndExcludeSensitiveInputs(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "src"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"src/parser.go", ".env.local"} {
		if err := os.WriteFile(filepath.Join(root, path), []byte("fixture\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "outside")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, ".env.local"), filepath.Join(root, "src", "alias.go")); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, tool, path, want string }{
		{"read", "Read", filepath.Join(root, "src/parser.go"), "src/parser.go"},
		{"new-file", "Write", "src/new.go", "src/new.go"},
		{"absent-read", "Read", "RAW_PATH_FIXTURE", ""},
		{"secret", "Read", ".env.local", ""},
		{"secret-alias", "Read", "src/alias.go", ""},
		{"outside", "Write", "outside/new.go", ""},
		{"traversal", "Read", "../RAW_PRIVATE_PATH_FIXTURE", ""},
		{"control", "Read", "src/parser.go\nRAW_TEXT_FIXTURE", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := map[string]any{"type": "assistant", "message": map[string]any{"content": []any{map[string]any{"type": "tool_use", "name": tc.tool,
				"input": map[string]any{"file_path": tc.path, "pattern": "RAW_SEARCH_FIXTURE", "command": "RAW_COMMAND_FIXTURE", "content": "RAW_TEXT_FIXTURE"}}}}}
			got := ActivityEvents("claude", "audit", root, event)
			if len(got) != 1 || got[0].Path != tc.want || got[0].Status != "started" {
				t.Fatalf("activity=%+v, want path=%q", got, tc.want)
			}
		})
	}
	command := ActivityEvents("codex", "execute", root, map[string]any{"type": "item.completed", "item": map[string]any{
		"type": "command_execution", "command": "RAW_COMMAND_FIXTURE", "aggregated_output": "RAW_OUTPUT_FIXTURE", "exit_code": float64(1)}})
	if len(command) != 1 || command[0].Action != "command" || command[0].Status != "failed" || command[0].Path != "" {
		t.Fatal("command activity", command)
	}
	if got := ActivityEvents("codex", "execute", root, map[string]any{"type": "item.completed", "item": map[string]any{"type": "agent_message", "text": "RAW_PROSE_FIXTURE"}}); len(got) != 0 {
		t.Fatal("prose generated an activity", got)
	}
}
