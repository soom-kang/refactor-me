package controller

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/soom-kang/refactor-me/tool/go/internal/surface"
	"github.com/soom-kang/refactor-me/tool/go/internal/workspace"
)

type heartbeatCapture struct {
	bytes.Buffer
	writes chan struct{}
}

func (w *heartbeatCapture) Write(data []byte) (int, error) {
	n, err := w.Buffer.Write(data)
	if bytes.Contains(data, []byte("경과)")) {
		w.writes <- struct{}{}
	}
	return n, err
}

func TestProgressHeartbeatStopsBeforeNextStageAndReturn(t *testing.T) {
	output := &heartbeatCapture{writes: make(chan struct{})}
	p := newProgress(output, "ko")
	start := time.Unix(100, 0)
	p.now = func() time.Time { return start }
	first, second := make(chan time.Time), make(chan time.Time)
	tickerCalls, cancelled := 0, 0
	p.ticker = func() (<-chan time.Time, func()) {
		tickerCalls++
		ch := first
		if tickerCalls == 2 {
			ch = second
		}
		return ch, func() { cancelled++ }
	}
	p.begin("First stage", "첫 단계")
	first <- start.Add(30 * time.Second)
	<-output.writes
	// begin joins the old heartbeat before writing the next stage.
	p.begin("Second stage", "다음 단계")
	second <- start.Add(60 * time.Second)
	<-output.writes
	p.stop()
	p.stop()
	before := output.String()
	select {
	case second <- start.Add(90 * time.Second):
		t.Fatal("heartbeat still listening after stop")
	default:
	}
	if output.String() != before || cancelled != 2 {
		t.Fatal("output after return or ticker not stopped", cancelled)
	}
	for _, wanted := range []string{"첫 단계", "계속 진행 중입니다: 첫 단계 (30s 경과)", "다음 단계", "계속 진행 중입니다: 다음 단계 (1m0s 경과)"} {
		if !strings.Contains(before, wanted) {
			t.Fatal("missing", wanted, before)
		}
	}
	if strings.Contains(before[strings.Index(before, "다음 단계"):], "첫 단계") {
		t.Fatal("old stage heartbeat after transition", before)
	}
}

func TestProgressSerializesHeartbeatAndOtherRunWriters(t *testing.T) {
	var output bytes.Buffer
	p := newProgress(&output, "en")
	ticks := make(chan time.Time, 100)
	p.ticker = func() (<-chan time.Time, func()) { return ticks, func() {} }
	p.begin("Validation", "검증")
	var writers sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		writers.Go(func() {
			for range 100 {
				// Simulates direct doctor cleanup writes and event messages.
				_, _ = p.writer.Write([]byte("other run writer\n"))
				p.line("event message", "event message")
			}
		})
	}
	for range 100 {
		ticks <- time.Now().Add(time.Minute)
	}
	writers.Wait()
	p.stop()
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		if line != "Validation" && line != "other run writer" && line != "event message" && !strings.HasPrefix(line, "Still working: Validation (") {
			t.Fatal("interleaved line", line)
		}
	}
}

func TestProgressOnlyReportsSuccessfullySavedOutcomes(t *testing.T) {
	var output bytes.Buffer
	r := &runner{surface: surface.Context{Stderr: &output, Args: surface.Args{Language: "ko"}}, runDir: t.TempDir(), candidateLabel: "parseLegacy",
		state: newState("r", "repo", "wt", "base", "main", "result", nil, nil)}
	if err := r.fail("fp", "EXECUTE_REJECTED", "private model notes\nrun secret command", nil, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "EXECUTE_REJECTED") || !strings.Contains(output.String(), filepath.Join(r.runDir, "state.json")) || strings.Contains(output.String(), "private model notes") {
		t.Fatal(output.String())
	}
	data, err := os.ReadFile(filepath.Join(r.runDir, "state.json"))
	if err != nil || !bytes.Contains(data, []byte("private model notes")) {
		t.Fatal("original evidence not saved", err)
	}
	output.Reset()
	r.runDir = filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(r.runDir, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := r.saveSkipped("NO_OP"); err == nil || output.Len() != 0 {
		t.Fatal("claimed saved outcome after failed save", err, output.String())
	}
	r.diagnostic("state.json", "execution.json")
	if !strings.Contains(output.String(), "저장하지 못했습니다") {
		t.Fatal("missing unsaved-record notice", output.String())
	}
}

func TestCommitAndRollbackDoNotClaimSuccessOnFailure(t *testing.T) {
	for _, failure := range []string{"commit-hook", "publication", "state-save"} {
		t.Run(failure, func(t *testing.T) {
			repo := t.TempDir()
			gitTest(t, repo, "init", "-q")
			gitTest(t, repo, "config", "user.name", "Test")
			gitTest(t, repo, "config", "user.email", "test@example.invalid")
			if err := os.WriteFile(filepath.Join(repo, "a.go"), []byte("before\n"), 0600); err != nil {
				t.Fatal(err)
			}
			gitTest(t, repo, "add", "a.go")
			gitTest(t, repo, "commit", "-qm", "base")
			base := gitTest(t, repo, "rev-parse", "HEAD")
			if err := os.WriteFile(filepath.Join(repo, "a.go"), []byte("after\n"), 0600); err != nil {
				t.Fatal(err)
			}
			gitTest(t, repo, "add", "a.go")
			tree := gitTest(t, repo, "write-tree")
			var output bytes.Buffer
			r := &runner{ctx: context.Background(), surface: surface.Context{Repo: repo, Stderr: &output}, wt: repo, runDir: t.TempDir(), candidateLabel: "A",
				state: newState("r", repo, repo, base, "main", "result", nil, nil), gateFacts: workspace.GateFacts{Changed: []string{"a.go"}, ProspectiveTree: tree}}
			defer r.endProgress()
			if failure == "publication" {
				gitTest(t, repo, "branch", "result", base)
			}
			if failure == "commit-hook" || failure == "state-save" {
				hook := "#!/bin/sh\nexit 1\n"
				if failure == "state-save" {
					// Keep the approved tree intact but prevent the post-commit state save.
					hook = "#!/bin/sh\nmv '" + filepath.Join(r.runDir, "state.json") + "' '" + filepath.Join(r.runDir, "old-state.json") + "'\nmkdir '" + filepath.Join(r.runDir, "state.json") + "'\n"
				}
				if err := os.WriteFile(filepath.Join(repo, ".git", "hooks", "pre-commit"), []byte(hook), 0700); err != nil {
					t.Fatal(err)
				}
			}
			err := r.commit(map[string]any{"title": "remove old code", "fp": "fp"}, contractCandidate(nil), t.TempDir())
			if err == nil || strings.Contains(output.String(), "Commit:") {
				t.Fatal("claimed commit completion", err, output.String())
			}
		})
	}
	var output bytes.Buffer
	r := &runner{surface: surface.Context{Repo: t.TempDir(), Stderr: &output}, wt: t.TempDir()}
	if err := r.rollback("invalid"); err == nil || strings.Contains(output.String(), "Rolled back") {
		t.Fatal("claimed rollback completion", err, output.String())
	}
}
