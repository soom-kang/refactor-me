package workspace

import (
	"context"
	"reflect"
	"testing"
)

func TestBaselineObserverReportsActualStatus(t *testing.T) {
	for _, tc := range []struct {
		name   string
		argv   []string
		status string
	}{
		{"green", []string{"/bin/sh", "-c", "exit 0"}, StatusGreen},
		{"red", []string{"/bin/sh", "-c", "echo src/a.go:1; exit 1"}, StatusRed},
		{"timeout", []string{"/bin/sh", "-c", "sleep 10"}, StatusTimeout},
		{"unrunnable", []string{"missing-refactor-observer-fixture"}, StatusUnrunnable},
		{"opaque", []string{"/bin/sh", "-c", "echo unknown failure; exit 1"}, StatusOpaque},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Command{ID: tc.name, Area: ".", Cwd: ".", Tier: TierFast, Name: "check", Argv: tc.argv, TimeoutMS: 5000}
			if tc.status == StatusTimeout {
				c.TimeoutMS = 50
			}
			var events []CommandEvent
			baseline := RunBaseline(context.Background(), []Command{c}, t.TempDir(), func(e CommandEvent) { events = append(events, e) })
			if len(events) != 2 || events[0].Kind != "started" || events[1].Kind != "completed" {
				t.Fatalf("events=%+v", events)
			}
			for _, e := range events {
				if !e.Baseline || !reflect.DeepEqual(e.Command, c) {
					t.Fatal("baseline event lost command or phase", e)
				}
			}
			if events[1].Result.Status != tc.status || !reflect.DeepEqual(events[1].Result, baseline.Results[0]) {
				t.Fatalf("completed result=%+v, baseline=%+v", events[1].Result, baseline.Results)
			}
		})
	}
}

func TestLadderObserverReportsFinalComparison(t *testing.T) {
	for _, tc := range []struct {
		name      string
		body      string
		base      string
		signature []string
		status    string
		ok        bool
		failure   string
	}{
		{"green", "exit 0", StatusGreen, nil, StatusGreen, true, ""},
		{"existing-red", "echo src/a.go:1; exit 1", StatusRed, []string{"src/a.go:1"}, StatusRed, true, ""},
		{"red-regression", "echo src/b.go:1; exit 1", StatusRed, []string{"src/a.go:1"}, StatusRed, false, "REGRESSION"},
		{"green-regression", "echo src/a.go:1; exit 1", StatusGreen, nil, StatusRed, false, "REGRESSION"},
		{"timeout", "sleep 10", StatusGreen, nil, StatusTimeout, false, "VALIDATION_TIMEOUT"},
		{"opaque-regression", "echo unknown failure; exit 1", StatusRed, []string{"src/a.go:1"}, StatusOpaque, false, "REGRESSION"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Command{ID: tc.name, Area: ".", Cwd: ".", Tier: TierFast, Name: "check", Argv: []string{"/bin/sh", "-c", tc.body}, TimeoutMS: 5000}
			if tc.status == StatusTimeout {
				c.TimeoutMS = 50
			}
			base := Baseline{Results: []CommandResult{{Command: c, Status: tc.base, Signature: tc.signature}}}
			var events []CommandEvent
			ladder := RunLadder(context.Background(), base, []Command{c}, t.TempDir(), []string{"src/a.go"}, []string{"."}, func(e CommandEvent) { events = append(events, e) })
			if len(events) != 2 || events[0].Kind != "started" || events[1].Kind != "completed" {
				t.Fatalf("events=%+v", events)
			}
			for _, e := range events {
				if e.Baseline || !reflect.DeepEqual(e.Command, c) {
					t.Fatal("ladder event lost command or phase", e)
				}
			}
			final := events[1].Result
			if final.Status != tc.status || final.OK != tc.ok || final.FailureKind != tc.failure || ladder.OK != tc.ok {
				t.Fatalf("final=%+v, ladder=%+v", final, ladder)
			}
			if !reflect.DeepEqual(final, ladder.Checks[0]) {
				t.Fatal("observer received pre-comparison result")
			}
		})
	}
}

func TestLadderObserverDoesNotStartSkippedOrSubsequentChecks(t *testing.T) {
	commands := []Command{
		{ID: "other-area", Area: "other", Cwd: "other"},
		{ID: "baseline-timeout", Area: "."},
		{ID: "baseline-unrunnable", Area: "."},
		{ID: "baseline-opaque", Area: "."},
		{ID: "regression", Area: ".", Cwd: ".", Argv: []string{"/bin/sh", "-c", "exit 1"}},
		{ID: "after-failure", Area: ".", Cwd: ".", Argv: []string{"/bin/sh", "-c", "exit 0"}},
	}
	baseline := Baseline{Results: []CommandResult{
		{Command: commands[1], Status: StatusTimeout},
		{Command: commands[2], Status: StatusUnrunnable},
		{Command: commands[3], Status: StatusOpaque},
		{Command: commands[4], Status: StatusGreen},
		{Command: commands[5], Status: StatusGreen},
	}}
	var events []CommandEvent
	ladder := RunLadder(context.Background(), baseline, commands, t.TempDir(), []string{"src/a.go"}, []string{".", "other"}, func(e CommandEvent) { events = append(events, e) })
	if ladder.OK || len(events) != 2 || events[0].Command.ID != "regression" || events[1].Command.ID != "regression" {
		t.Fatalf("skipped checks produced events: %+v, ladder=%+v", events, ladder)
	}
}
