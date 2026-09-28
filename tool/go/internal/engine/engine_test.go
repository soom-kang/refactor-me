package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestProviderArguments(t *testing.T) {
	read := strings.Join(ClaudeArgs("read", map[string]any{"type": "object"}, "", "", 0), " ")
	for _, want := range []string{"--tools Skill,Read,Glob,Grep", "--permission-mode plan", "--disallowed-tools Edit Write NotebookEdit Bash WebFetch WebSearch", "--setting-sources project"} {
		if !strings.Contains(read, want) {
			t.Fatalf("missing %q: %s", want, read)
		}
	}
	write := strings.Join(ClaudeArgs("write", nil, "", "", 0), " ")
	if !strings.Contains(write, "--tools Skill,Read,Glob,Grep,Edit,Write") || !strings.Contains(write, "--disallowed-tools Bash") {
		t.Fatalf("unsafe write args: %s", write)
	}
	codex := CodexArgs("read", "/w", "/s", "/o", "m", "high")
	count := 0
	for _, arg := range codex {
		if arg == "-c" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("codex overrides=%d", count)
	}
	if strings.Contains(strings.Join(codex, " "), "--skip-git-repo-check") {
		t.Fatal("git check bypassed")
	}
}
func TestTaxonomy(t *testing.T) {
	claude := processResult{stdout: `{"type":"result","subtype":"success","is_error":true,"result":"Failed to authenticate: OAuth session expired"}`, stderr: ""}
	if got := classifyClaude(claude, true).failure; got != "AUTH" {
		t.Fatalf("claude auth=%s", got)
	}
	claude.stdout = `{"type":"result","is_error":true,"result":"usage limit reached"}`
	if got := classifyClaude(claude, false); got.failure != "QUOTA" || !got.hard {
		t.Fatalf("hard quota=%+v", got)
	}
	claude.stdout = `{"type":"result","is_error":false,"result":"x"}`
	if got := classifyClaude(claude, true).failure; got != "SCHEMA" {
		t.Fatalf("missing structured output=%s", got)
	}
	z := 0
	codex := processResult{stdout: `{"type":"item.completed","item":{"type":"error","message":"Skill descriptions were shortened"}}`, exitCode: &z}
	out := filepath.Join(t.TempDir(), "last.json")
	if err := os.WriteFile(out, []byte(`{"ok":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if got := classifyCodex(codex, out, true).failure; got != "OK" {
		t.Fatalf("benign JSONL warning=%s", got)
	}
	one := 1
	codex = processResult{stderr: "failed to parse schema file", exitCode: &one}
	if got := classifyCodex(codex, out, true); got.failure != "PROCESS" || !got.fatal {
		t.Fatalf("schema rejection=%+v", got)
	}
}
func TestSchemaAndPrompts(t *testing.T) {
	schema := Schema("audit")
	if schema == nil {
		t.Fatal("missing audit schema")
	}
	if errs := Validate(schema, map[string]any{"schema_version": "1"}); len(errs) == 0 {
		t.Fatal("missing required fields passed")
	}
	b, err := BuildPrompt("audit", PromptArgs{Provider: "codex", Nonce: "0123456789abcdef", RepoFacts: "tracked source files: 1", Targets: []string{"app/web"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"$sharpen-clarify", "app/web", "<<<REPO_FACTS id=0123456789abcdef>>>"} {
		if !strings.Contains(b, want) {
			t.Fatalf("prompt lacks %q", want)
		}
	}
	if strings.Contains(b, "__REPO_FACTS__") {
		t.Fatal("unexpanded marker")
	}
	packet := map[string]any{"category": "DEDUPLICATION", "stop_conditions": []any{"outside scope"}}
	b, err = BuildPrompt("execute", PromptArgs{Provider: "claude", Packet: packet})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b, "sharpen-dedupe") || !strings.Contains(b, "  - outside scope") {
		t.Fatal("execute prompt lost dedupe or stop condition")
	}
}
func TestCallProviderTimeoutKillsGroup(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-claude")
	body := "#!/bin/sh\nsleep 30 &\necho $! > child.pid\nwait\n"
	if err := os.WriteFile(script, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := Request{Phase: "audit", CWD: dir, RunDir: dir, Body: "x", Timeout: 30 * time.Second}
	type answer struct {
		result Result
		err    error
	}
	resultCh := make(chan answer, 1)
	go func() {
		result, err := CallProvider(ctx, "claude", req, Config{Agents: map[string]AgentConfig{"claude": {Bin: script}}}, nil)
		resultCh <- answer{result, err}
	}()
	pidFile := filepath.Join(dir, "child.pid")
	deadline := time.Now().Add(5 * time.Second)
	var pidText []byte
	for time.Now().Before(deadline) {
		var err error
		pidText, err = os.ReadFile(pidFile)
		if err == nil {
			break
		}
		select {
		case got := <-resultCh:
			t.Fatalf("provider exited before readiness: %+v, %v", got.result, got.err)
		default:
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(pidText) == 0 {
		t.Fatal("fake provider never reported child readiness")
	}
	var pid int
	if _, err := fmt.Sscan(strings.TrimSpace(string(pidText)), &pid); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case got := <-resultCh:
		if got.err != nil {
			t.Fatal(got.err)
		}
		if got.result.Failure != "TIMEOUT" {
			t.Fatalf("failure=%s", got.result.Failure)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not terminate the process group promptly")
	}
	if err := syscall.Kill(pid, 0); err == nil {
		t.Fatalf("child %d survived process group cancellation", pid)
	}
}
func TestWritePrivilegeFailsClosed(t *testing.T) {
	_, err := CallProvider(context.Background(), "codex", Request{Phase: "doctor", Mode: "write", CWD: t.TempDir(), RunDir: t.TempDir()}, Config{}, nil)
	if err == nil || !strings.Contains(err.Error(), "not in WRITE_PHASES") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCollectRepoFactsAndAreaSkills(t *testing.T) {
	wt := t.TempDir()
	init := exec.Command("git", "init", "-q", wt)
	if output, err := init.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	for name, body := range map[string]string{
		"app/api/main.go":  "package main\nfunc main() {}\n",
		"app/web/index.ts": "export const x = 1\n",
		"vendor/lib.js":    "export const ignored = true\n",
	} {
		path := filepath.Join(wt, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	add := exec.Command("git", "add", ".")
	add.Dir = wt
	if output, err := add.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v: %s", err, output)
	}
	facts, err := CollectRepoFacts(wt, FactsOptions{Areas: []string{"app"}, Targets: []string{"app/api"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(facts, "app/api/main.go") || strings.Contains(facts, "app/web/index.ts") || strings.Contains(facts, "vendor/lib.js") {
		t.Fatalf("wrong scoped inventory: %s", facts)
	}
	if !strings.Contains(facts, "The other 1 tracked source file(s)") {
		t.Fatal("out-of-scope count absent")
	}
	root := filepath.Join(wt, "app", "api", ".agents", "skills", "backend")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\ndescription: Backend review procedure\n---\n"), 0600); err != nil {
		t.Fatal(err)
	}
	area := CollectAreaSkills(wt, []string{"app/api/main.go"})
	if !strings.Contains(area, "backend") || !strings.Contains(area, "Backend review procedure") {
		t.Fatalf("area skill missing: %s", area)
	}
}

func TestCallProviderEscalatesToKill(t *testing.T) {
	oldGrace := processKillGrace
	processKillGrace = 100 * time.Millisecond
	defer func() { processKillGrace = oldGrace }()
	dir := t.TempDir()
	script := filepath.Join(dir, "ignores-term")
	body := "#!/bin/sh\ntrap '' TERM\nsleep 30 &\necho $! > child.pid\nwait\n"
	if err := os.WriteFile(script, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resultCh := make(chan Result, 1)
	go func() {
		result, _ := CallProvider(ctx, "claude", Request{Phase: "audit", CWD: dir, RunDir: dir, Timeout: 30 * time.Second}, Config{Agents: map[string]AgentConfig{"claude": {Bin: script}}}, nil)
		resultCh <- result
	}()
	pidFile := filepath.Join(dir, "child.pid")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(pidFile); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(pidFile); err != nil {
		t.Fatal("fake provider never spawned child")
	}
	started := time.Now()
	cancel()
	select {
	case result := <-resultCh:
		if result.Failure != "TIMEOUT" {
			t.Fatalf("failure=%s", result.Failure)
		}
		if time.Since(started) < processKillGrace/2 {
			t.Fatal("process exited before SIGKILL escalation")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SIGKILL escalation did not complete")
	}
}
