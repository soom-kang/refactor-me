package controller

import (
	"context"
	"encoding/json"
	"github.com/soom-kang/refactor-me/tool/go/internal/engine"
	"github.com/soom-kang/refactor-me/tool/go/internal/surface"
	"github.com/soom-kang/refactor-me/tool/go/internal/workspace"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

func migrationCandidate(overrides map[string]any) map[string]any {
	c := map[string]any{"candidate_id": "c", "category": "DEAD_CODE", "risk_level": "L0_LOW", "readiness": "READY", "related_files": []any{"src/a.go"}, "estimated_file_count": 1, "primary_symbol": "A", "problem": "missing evidence"}
	for k, v := range overrides {
		c[k] = v
	}
	return c
}
func migrationPolicy() policy {
	return policy{AllowedRisks: []string{"L0_LOW", "L1_MODERATE", "L2_HIGH"}, UnknownRisk: "set_aside", MaxAttemptsPerFingerprint: 2, MaxFilesPerCandidate: 8}
}

func TestMigrationRankContracts(t *testing.T) {
	for _, tc := range []struct {
		name      string
		overrides map[string]any
		reason    string
	}{
		{"critical", map[string]any{"risk_level": "L3_CRITICAL"}, "RISK_EXCLUDED"},
		{"unknown", map[string]any{"risk_level": "UNKNOWN"}, "RISK_UNKNOWN"},
		{"model-reject", map[string]any{"readiness": "REJECT"}, "MODEL_REJECTED"},
		{"oversized", map[string]any{"estimated_file_count": 9}, "TOO_LARGE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			eligible, rejected := rank([]map[string]any{migrationCandidate(tc.overrides)}, nil, migrationPolicy(), nil, nil)
			if len(eligible) != 0 || len(rejected) != 1 || rejected[0].Reason != tc.reason {
				t.Fatalf("%v %+v", eligible, rejected)
			}
			if tc.reason == "RISK_UNKNOWN" && !strings.Contains(rejected[0].Detail, "missing evidence") {
				t.Fatal(rejected)
			}
		})
	}
	p := migrationPolicy()
	p.AllowedRisks = append(p.AllowedRisks, "UNKNOWN")
	c := migrationCandidate(map[string]any{"risk_level": "UNKNOWN"})
	if eligible, _ := rank([]map[string]any{c}, nil, p, nil, nil); len(eligible) != 0 {
		t.Fatal("UNKNOWN widened")
	}
	p.UnknownRisk = "deep_check"
	for _, readiness := range []string{"READY", "REJECT", "NEEDS_EVIDENCE"} {
		c := migrationCandidate(map[string]any{"risk_level": "UNKNOWN", "readiness": readiness})
		eligible, rejected := rank([]map[string]any{c}, nil, p, nil, nil)
		if readiness == "NEEDS_EVIDENCE" {
			if len(eligible) != 1 || len(rejected) != 0 {
				t.Fatal(eligible, rejected)
			}
		} else if len(rejected) != 1 || rejected[0].Reason != "RISK_UNKNOWN" {
			t.Fatal(eligible, rejected)
		}
	}
	for _, tc := range []struct {
		paths   []any
		targets []string
		want    bool
	}{
		{[]any{"app/web/a.ts", "outside.go"}, []string{"app/web"}, true},
		{[]any{"app/website/a.ts"}, []string{"app/web"}, false},
		{[]any{"outside.go"}, nil, true},
	} {
		eligible, _ := rank([]map[string]any{migrationCandidate(map[string]any{"related_files": tc.paths})}, tc.targets, p, nil, nil)
		if (len(eligible) == 1) != tc.want {
			t.Fatal(tc, eligible)
		}
	}
}

func TestMigrationRankSeenAndOrder(t *testing.T) {
	c := migrationCandidate(nil)
	fp := fingerprint(c)
	reworded := migrationCandidate(map[string]any{"title": "new prose", "candidate_id": "other"})
	if fingerprint(reworded) != fp {
		t.Fatal("prose changes identity")
	}
	for _, tc := range []struct {
		seen     map[string]bool
		attempts map[string]int
		reason   string
	}{{map[string]bool{fp: true}, nil, "ALREADY_SEEN"}, {nil, map[string]int{fp: 2}, "ATTEMPTS_EXHAUSTED"}, {nil, map[string]int{fp: 1}, ""}} {
		eligible, rejected := rank([]map[string]any{reworded}, nil, migrationPolicy(), tc.seen, tc.attempts)
		if tc.reason == "" {
			if len(eligible) != 1 {
				t.Fatal(rejected)
			}
		} else if len(rejected) != 1 || rejected[0].Reason != tc.reason {
			t.Fatal(rejected)
		}
	}
	p := migrationPolicy()
	p.UnknownRisk = "deep_check"
	out, _ := rank([]map[string]any{
		migrationCandidate(map[string]any{"candidate_id": "big", "estimated_file_count": 5}),
		migrationCandidate(map[string]any{"candidate_id": "risky", "risk_level": "L2_HIGH"}),
		migrationCandidate(map[string]any{"candidate_id": "small"}),
		migrationCandidate(map[string]any{"candidate_id": "unsure", "readiness": "NEEDS_EVIDENCE"}),
		migrationCandidate(map[string]any{"candidate_id": "unknown", "risk_level": "UNKNOWN", "readiness": "NEEDS_EVIDENCE"}),
	}, nil, p, nil, nil)
	var ids []string
	for _, x := range out {
		ids = append(ids, str(x["candidate_id"]))
	}
	if !reflect.DeepEqual(ids, []string{"small", "big", "unsure", "risky", "unknown"}) {
		t.Fatal(ids)
	}
}

func TestMigrationPolicyAndHistory(t *testing.T) {
	for _, mode := range []string{"set_aside", "deep_check", "deep-check"} {
		_, err := readPolicy(map[string]any{"policy": map[string]any{"unknown_risk": mode}})
		if (err != nil) != (mode == "deep-check") {
			t.Fatal(mode, err)
		}
	}
	state := newState("id", "root", "worktree", "base", "main", "result", nil, []string{"codex"})
	if state.ToolVersion != "dev" || state.Schema != 2 {
		t.Fatal(state)
	}
	state.markSkipped("a", "TOO_LARGE", "details", []string{"src/a.go"}, false)
	if got := stringsOf(state.Seen.Skipped[0].Detail["paths"]); !reflect.DeepEqual(got, []string{"src/a.go"}) {
		t.Fatal(got)
	}
	state.Seen.Skipped = append(state.Seen.Skipped, skippedCandidate{FP: "legacy", Reason: "OUT_OF_SCOPE", Detail: map[string]any{"paths": []string{"legacy.go"}}})
	r := runner{state: state}
	if got := r.violatedPaths(); len(got) != 0 {
		t.Fatal(got)
	}
	state.markSkipped("b", "FORBIDDEN_PATH", "details", []string{"go.mod"}, true)
	if got := r.violatedPaths(); !reflect.DeepEqual(got, []string{"go.mod"}) {
		t.Fatal(got)
	}
	if !state.seen("a") || state.seen("absent") {
		t.Fatal("seen state")
	}
	for _, tc := range []struct {
		kinds []string
		want  string
	}{{[]string{"ALL_FILTERED", "ALL_FILTERED"}, "every proposed candidate was filtered by policy"}, {[]string{"NO_PROPOSALS"}, "the audit proposed no candidates"}, {nil, "no eligible candidates remain"}, {[]string{"NO_PROPOSALS", "ALL_FILTERED"}, "no eligible candidates remain"}} {
		if got := noCandidatesReason(tc.kinds); got != tc.want {
			t.Fatal(got)
		}
	}
}

func TestMigrationPacketRiskGate(t *testing.T) {
	for _, risk := range []string{"UNKNOWN", "L3_CRITICAL"} {
		t.Run(risk, func(t *testing.T) {
			repo := t.TempDir()
			gitTest(t, repo, "init", "-q")
			raw, err := os.ReadFile(filepath.Join("..", "..", "..", "WORKFLOW.md"))
			if err != nil {
				t.Fatal(err)
			}
			match := regexp.MustCompile("(?s)<!-- example: deepcheck-success schema: \\w+ -->\\n```json\\n(.*?)\\n```").FindSubmatch(raw)
			if len(match) != 2 {
				t.Fatal("missing packet example")
			}
			var packet map[string]any
			if err := json.Unmarshal(match[1], &packet); err != nil {
				t.Fatal(err)
			}
			packet["risk_level"] = risk
			fixture := filepath.Join(t.TempDir(), "packet.json")
			encoded, err := json.Marshal(packet)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(fixture, encoded, 0600); err != nil {
				t.Fatal(err)
			}
			bin := filepath.Join(t.TempDir(), "fake-codex")
			body := "#!/bin/sh\nout=''\nwhile [ $# -gt 0 ]; do\n if [ \"$1\" = '--output-last-message' ]; then shift; out=$1; fi\n shift\ndone\ncp '" + fixture + "' \"$out\"\n"
			if err := os.WriteFile(bin, []byte(body), 0700); err != nil {
				t.Fatal(err)
			}
			runDir := t.TempDir()
			p := migrationPolicy()
			p.UnknownRisk = "deep_check"
			p.MaxWallClockMin = 10
			p.MaxCommits = 10
			p.MaxCycles = 10
			r := runner{ctx: context.Background(), surface: surface.Context{Repo: repo, Stderr: io.Discard}, wt: repo, runDir: runDir, policy: p, state: newState("r", repo, repo, "", "main", "result", nil, []string{"codex"}), started: time.Now(), engineConfig: engine.Config{Agents: map[string]engine.AgentConfig{"codex": {Bin: bin}}}}
			c := migrationCandidate(map[string]any{"fp": "fp"})
			_, ok, err := r.selectPacket(c, runDir)
			if err != nil || ok {
				t.Fatal(ok, err)
			}
			want := "RISK_EXCLUDED"
			if risk == "UNKNOWN" {
				want = "RISK_UNKNOWN"
			}
			if len(r.state.Seen.Skipped) != 1 || r.state.Seen.Skipped[0].Reason != want {
				t.Fatal(r.state.Seen.Skipped)
			}
		})
	}
}

func TestMigrationDeclaredDeletionRefusal(t *testing.T) {
	repo := t.TempDir()
	gitTest(t, repo, "init", "-q")
	gitTest(t, repo, "config", "user.name", "Test")
	gitTest(t, repo, "config", "user.email", "test@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "keep.go"), []byte("package keep\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repo, "add", ".")
	gitTest(t, repo, "commit", "-qm", "fixture")
	base := gitTest(t, repo, "rev-parse", "HEAD")
	wt := filepath.Join(t.TempDir(), "wt")
	if err := workspace.WorktreeAdd(repo, wt, base); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := workspace.WorktreeRemove(repo, wt); err != nil {
			t.Error(err)
		}
	}()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "WORKFLOW.md"))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile("(?s)<!-- example: execute-success schema: \\w+ -->\\n```json\\n(.*?)\\n```").FindSubmatch(raw)
	if len(match) != 2 {
		t.Fatal("missing execution example")
	}
	var answer map[string]any
	if err := json.Unmarshal(match[1], &answer); err != nil {
		t.Fatal(err)
	}
	answer["changed_files"] = []any{}
	answer["deleted_files"] = []any{"keep.go"}
	encoded, err := json.Marshal(answer)
	if err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(t.TempDir(), "answer.json")
	if err := os.WriteFile(fixture, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "fake-codex")
	script := "#!/bin/sh\nout=''\nwhile [ $# -gt 0 ]; do\n if [ \"$1\" = '--output-last-message' ]; then shift; out=$1; fi\n shift\ndone\ncp '" + fixture + "' \"$out\"\n"
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	p := migrationPolicy()
	p.MaxWallClockMin = 10
	p.MaxCommits = 10
	p.MaxCycles = 10
	runDir := t.TempDir()
	r := runner{ctx: context.Background(), surface: surface.Context{Repo: repo, Stderr: io.Discard}, wt: wt, runDir: runDir, policy: p, state: newState("r", repo, wt, base, "main", "result", nil, []string{"codex"}), started: time.Now(), engineConfig: engine.Config{Agents: map[string]engine.AgentConfig{"codex": {Bin: bin}}}}
	packet := map[string]any{"fp": "fp", "category": "DEAD_CODE", "allowlist": []any{"other.go"}}
	_, ok, err := r.execute(packet, runDir, base)
	if err != nil || ok {
		t.Fatal(ok, err)
	}
	for _, root := range []string{repo, wt} {
		if b, err := os.ReadFile(filepath.Join(root, "keep.go")); err != nil || string(b) != "package keep\n" {
			t.Fatal("refused deletion changed file", root, string(b), err)
		}
	}
}

func TestMigrationHandoffSnapshot(t *testing.T) {
	repo := t.TempDir()
	gitTest(t, repo, "init", "-q")
	bin := filepath.Join(t.TempDir(), "fake-codex")
	script := "#!/bin/sh\nout=''\nwhile [ $# -gt 0 ]; do\n if [ \"$1\" = '--output-last-message' ]; then shift; out=$1; fi\n shift\ndone\nprintf 'fixture handoff' > \"$out\"\n"
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	runDir := t.TempDir()
	r := runner{ctx: context.Background(), surface: surface.Context{Repo: repo, Stderr: io.Discard}, wt: repo, runDir: runDir, state: newState("r", repo, repo, "", "main", "result", nil, []string{"codex"}), engineConfig: engine.Config{Agents: map[string]engine.AgentConfig{"codex": {Bin: bin}}}}
	if err := r.writeHandoff("claude", "QUOTA"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(runDir, "handoff.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"not the final outcome", "report.md", "claude → codex", "fixture handoff"} {
		if !strings.Contains(string(data), want) {
			t.Fatal(string(data))
		}
	}
}
