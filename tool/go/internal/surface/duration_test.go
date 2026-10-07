package surface

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestMaxMinutesParseBoundsAndCommandScope(t *testing.T) {
	for _, value := range []string{"1", "60", strconv.FormatInt(maxRuntimeMinutes, 10)} {
		args, err := Parse([]string{"run", "--max-minutes", value})
		if err != nil || args.MaxMinutes < 1 {
			t.Fatalf("value=%s args=%+v error=%v", value, args, err)
		}
	}
	for _, values := range [][]string{nil, {""}, {"0"}, {"-1"}, {"1.5"}, {"abc"}, {"--json"}, {strconv.FormatInt(maxRuntimeMinutes+1, 10)}, {"9223372036854775808"}} {
		if _, err := Parse(append([]string{"run", "--max-minutes"}, values...)); err == nil {
			t.Fatalf("accepted invalid limit %v", values)
		}
	}
	for _, command := range []string{"init", "doctor", "report", "clean", "version"} {
		if _, err := Parse([]string{command, "--max-minutes", "60"}); err == nil {
			t.Fatalf("accepted run option for %s", command)
		}
	}
}

func TestRunTimeSelectionPrecedencePreservesProjectConfig(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		options      []string
		interactive  bool
		cancel       bool
		want         int
		prompted     bool
	}{
		{name: "cli", source: "cli", options: []string{"--max-minutes", "60"}, interactive: true, want: 60},
		{name: "interactive", source: "interactive", interactive: true, want: 360, prompted: true},
		{name: "json", source: "config", options: []string{"--json"}, interactive: true, want: 270},
		{name: "noninteractive", source: "config", want: 270},
		{name: "cancel", interactive: true, cancel: true, prompted: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := testRepo(t)
			if err := os.MkdirAll(filepath.Join(repo, ".refactor"), 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(repo, ".refactor", "config.json")
			before := []byte(`{"schema_version":2,"policy":{"max_wall_clock_min":270,"max_commits":7},"future":{"keep":true}}` + "\n")
			if err := os.WriteFile(path, before, 0600); err != nil {
				t.Fatal(err)
			}
			called, prompted := false, false
			callbacks := Callbacks{Run: func(c Context) (RunResult, error) {
				called = true
				if c.RunLimitSource != tc.source || object(c.Config["policy"])["max_wall_clock_min"] != tc.want ||
					object(c.Config["policy"])["max_commits"] != float64(7) || object(c.Config["future"])["keep"] != true {
					t.Fatalf("incorrect runtime config/source: %+v", c)
				}
				return RunResult{Status: "NO_CHANGES", RunID: "test", RunDir: t.TempDir(), Report: []byte("{}\n")}, nil
			}}
			if tc.interactive {
				callbacks.SelectMaxMinutes = func(defaultMinutes int, language string) (int, error) {
					prompted = true
					if defaultMinutes != 270 || language != "ko" {
						t.Fatal("wrong prompt defaults", defaultMinutes, language)
					}
					if tc.cancel {
						return 0, errors.New("cancelled")
					}
					return 360, nil
				}
			}
			args := append([]string{"run", "--model", "fixture-model", "--fallback", "none", "--lang", "ko"}, tc.options...)
			var stdout, stderr bytes.Buffer
			code := Execute(args, repo, &stdout, &stderr, callbacks)
			if prompted != tc.prompted || called == tc.cancel || (code != ExitOK && !tc.cancel) || (tc.cancel && code != ExitAborted) {
				t.Fatalf("code=%d called=%v prompted=%v stderr=%s", code, called, prompted, stderr.String())
			}
			if tc.name == "json" && stdout.String() != "{}\n" {
				t.Fatal("non-JSON output", stdout.String())
			}
			if tc.cancel {
				for _, name := range []string{"runs", "lock.json", "last-run.json"} {
					if _, err := os.Stat(filepath.Join(repo, ".refactor", name)); !errors.Is(err, os.ErrNotExist) {
						t.Fatal("cancel created run artifacts", name, err)
					}
				}
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("project config changed", err)
			}
		})
	}
	cfg := DefaultConfig()
	before, _ := configuredMaxMinutes(cfg)
	if _, _, err := runDurationOptions(cfg, Args{Command: "run", MaxMinutes: 30}, nil); err != nil {
		t.Fatal(err)
	}
	if after, _ := configuredMaxMinutes(cfg); after != before {
		t.Fatal("input config map mutated")
	}
}

func TestMaxMinutesPromptPresetsCustomRetryAndCancellation(t *testing.T) {
	for _, language := range []string{"en", "ko"} {
		for _, tc := range []struct {
			input string
			want  int
		}{
			{"\n", 270}, {"30\n", 30}, {"60\n", 60}, {"180\n", 180}, {"360\n", 360},
			{"invalid\n60\n", 60}, {"custom\n0\n1.5\n999999999999999999999\n45\n", 45},
			{"q\n", 0}, {"custom\ncancel\n", 0}, {"", 0}, {"custom\n", 0},
		} {
			var output bytes.Buffer
			minutes, err := SelectMaxMinutes(strings.NewReader(tc.input), &output, 270, language)
			if minutes != tc.want || (err != nil) != (tc.want == 0) {
				t.Fatalf("lang=%s input=%q minutes=%d error=%v", language, tc.input, minutes, err)
			}
			if !strings.Contains(output.String(), "270") {
				t.Fatal("configured default not displayed", output.String())
			}
		}
	}
}

func TestInteractiveTerminalRejectsFilesAndPipes(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "not-a-terminal")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	defer write.Close()
	for _, pair := range [][2]*os.File{{file, file}, {read, write}, {nil, nil}, {os.Stdin, file}, {read, os.Stderr}} {
		if InteractiveTerminal(pair[0], pair[1]) {
			t.Fatal("nonterminal enabled prompting")
		}
	}
}
