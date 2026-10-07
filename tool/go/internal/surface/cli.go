package surface

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
)

const (
	ExitOK      = 0
	ExitAborted = 2
	ExitUnsafe  = 4
)

// Version is overridden by release builds. Local builds identify themselves as dev.
var Version = "dev"

// Commit identifies the source revision embedded by release builds.
var Commit = "unknown"

// LastRunSchemaVersion identifies the latest-run pointer format.
const LastRunSchemaVersion = 1

type Args struct {
	Command, Provider, Fallback, Language, ForceQuotaAt, Repo string
	Model, FallbackModel, Effort, FallbackEffort              string
	FallbackSet, JSON, Live, Version, LanguageSet             bool
	Targets                                                   []string
	MaxMinutes                                                int
}

type Context struct {
	Context            context.Context
	Repo               string
	Config             Config
	Args               Args
	Providers, Targets []string
	ProviderSettings   map[string]ProviderSettings
	RunLimitSource     string
	Stdout, Stderr     io.Writer
}

// OperationContext preserves the existing API for callers without cancellation.
func (c Context) OperationContext() context.Context {
	if c.Context != nil {
		return c.Context
	}
	return context.Background()
}

type RunResult struct {
	Status, RunID, RunDir, Branch string
	Report                        []byte
	Summary                       string
}

type DoctorResult struct {
	OK      bool
	Summary string
	JSON    []byte
}

type Callbacks struct {
	Run              func(Context) (RunResult, error)
	Doctor           func(Context) (DoctorResult, error)
	Clean            func(Context) (string, error)
	SelectMaxMinutes func(defaultMinutes int, language string) (int, error)
}

func Parse(argv []string) (Args, error) {
	args := Args{Command: "help", Language: "en", Live: true}
	var positional []string
	value := func(i *int, flag string) (string, error) {
		*i++
		if *i >= len(argv) || argv[*i] == "" || strings.HasPrefix(argv[*i], "-") {
			return "", fmt.Errorf("%s needs a value", flag)
		}
		return argv[*i], nil
	}
	for i := 0; i < len(argv); i++ {
		switch argv[i] {
		case "--repo":
			v, e := value(&i, "--repo")
			if e != nil {
				return args, e
			}
			args.Repo = v
		case "--target":
			v, e := value(&i, "--target")
			if e != nil {
				return args, e
			}
			args.Targets = append(args.Targets, v)
		case "--max-minutes":
			v, e := value(&i, "--max-minutes")
			if e != nil {
				return args, e
			}
			args.MaxMinutes, e = parseMaxMinutes(v)
			if e != nil {
				return args, fmt.Errorf("--max-minutes: %w", e)
			}
		case "--lang":
			v, e := value(&i, "--lang")
			if e != nil {
				return args, e
			}
			if v != "en" && v != "ko" {
				return args, fmt.Errorf("unsupported language: %s; use en or ko", v)
			}
			args.Language = v
			args.LanguageSet = true
		case "--provider":
			v, e := value(&i, "--provider")
			if e != nil {
				return args, e
			}
			args.Provider = v
		case "--fallback":
			v, e := value(&i, "--fallback")
			if e != nil {
				return args, e
			}
			args.Fallback = v
			args.FallbackSet = true
		case "--model", "--fallback-model", "--effort", "--fallback-effort":
			flag := argv[i]
			v, e := value(&i, flag)
			if e != nil {
				return args, e
			}
			if strings.TrimSpace(v) == "" {
				return args, fmt.Errorf("%s needs a nonblank value", flag)
			}
			switch flag {
			case "--model":
				args.Model = v
			case "--fallback-model":
				args.FallbackModel = v
			case "--effort":
				args.Effort = v
			case "--fallback-effort":
				args.FallbackEffort = v
			}
		case "--force-quota-at":
			v, e := value(&i, "--force-quota-at")
			if e != nil {
				return args, e
			}
			args.ForceQuotaAt = v
		case "--json":
			args.JSON = true
		case "--no-live-probe":
			args.Live = false
		case "--version":
			args.Version = true
		case "-h", "--help":
			args.Command = "help"
		default:
			if strings.HasPrefix(argv[i], "-") {
				return args, fmt.Errorf("unknown option: %s", argv[i])
			}
			positional = append(positional, argv[i])
		}
	}
	if len(positional) > 0 {
		args.Command = positional[0]
	}
	if len(positional) > 1 {
		return args, fmt.Errorf("unexpected argument: %s", positional[1])
	}
	if slices.Contains(argv, "--help") || slices.Contains(argv, "-h") {
		args.Command = "help"
		return args, nil
	}
	if len(positional) == 0 && len(argv) > 0 && !args.Version {
		return args, errors.New("a command is required; use refactor-me run to start refactoring")
	}
	if args.Repo != "" && !slices.Contains([]string{"init", "run", "doctor", "report", "clean"}, args.Command) {
		return args, errors.New("--repo is supported only for init, run, doctor, report and clean")
	}
	if args.LanguageSet && (args.Version || (args.Command != "run" && args.Command != "report")) {
		return args, errors.New("--lang is supported only for run and report")
	}
	if hasProviderOptions(args) && (args.Version || (args.Command != "run" && args.Command != "doctor")) {
		return args, errors.New("model and effort options are supported only for run and doctor")
	}
	if args.MaxMinutes != 0 && (args.Version || args.Command != "run") {
		return args, errors.New("--max-minutes is supported only for run")
	}
	return args, nil
}

const Help = `refactor-me — unattended behavior-preserving refactoring

USAGE
  refactor-me run [options]       audit → implement → validate → review → commit
  refactor-me doctor [options]    check preconditions only
  refactor-me report              print the most recent run's report
  refactor-me clean               remove finished worktrees
  refactor-me init [--repo path]   create optional project settings
  refactor-me version             print the tool version

OPTIONS
  --repo <path>            select a Git repository (default: current directory)
  --target <dir>            survey only inside <dir>; repeatable
                            relative to --repo root, or current directory without --repo
  --provider claude|codex   agent leading the run
  --fallback claude|codex|none
  --model <id>              model for the leading provider (overrides project config)
  --fallback-model <id>     model for the fallback provider
  --effort <level>          leading provider effort for every call, including doctor
  --fallback-effort <level> fallback provider effort for every call
  --max-minutes <n>         positive run time limit in minutes (run only; overrides config)
  --json                    machine-readable stdout
  --lang en|ko              progress, summary and report language (run/report; default: en)
  --no-live-probe           skip live provider checks

The source checkout, index, and HEAD are not edited. Accepted commits are
published to a local refactor/auto-* branch. The tool does not push or deploy.

Run and live doctor require a nonblank model for each selected provider,
from these options or agents.<provider>.model in .refactor/config.json.
Model and effort options apply only to run/doctor and are not saved.
Effort options override phase policy; omission preserves existing effort settings.
Run time selection is offered on macOS when stdin and stderr are terminals,
unless --max-minutes or --json is given. Enter keeps the configured limit.
The limit is checked between work units; an in-progress unit finishes first.

EXIT CODES
  0  completed or partially completed
  2  aborted
  4  halted unsafe; worktree retained
`

func VersionText(asJSON bool) string {
	source, _ := os.Executable()
	source, _ = filepath.Abs(source)
	if asJSON {
		data, _ := json.MarshalIndent(map[string]string{"name": "refactor-me", "version": Version, "go": runtime.Version(), "platform": runtime.GOOS, "arch": runtime.GOARCH, "source": source, "commit": Commit}, "", "  ")
		return string(data) + "\n"
	}
	return fmt.Sprintf("refactor-me %s\n  go      %s (%s/%s)\n  source  %s\n  commit  %s\n", Version, runtime.Version(), runtime.GOOS, runtime.GOARCH, source, Commit)
}

func ProviderOrder(cfg Config, args Args) ([]string, error) {
	primary := args.Provider
	if primary == "" {
		primary = ConfigString(cfg, "agents", "primary")
	}
	if primary == "" {
		primary = "codex"
	}
	if primary != "codex" && primary != "claude" {
		return nil, fmt.Errorf("unknown provider: %s", primary)
	}
	fallback := ConfigString(cfg, "agents", "fallback")
	if args.FallbackSet {
		fallback = args.Fallback
	}
	if fallback == "" || fallback == "none" || fallback == primary {
		return []string{primary}, nil
	}
	if fallback != "codex" && fallback != "claude" {
		return nil, fmt.Errorf("unknown fallback: %s", fallback)
	}
	return []string{primary, fallback}, nil
}

func RepoRoot(cwd string) (string, error) {
	cmd := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", errors.New("refactor-me must be run inside a git repository")
	}
	return filepath.EvalSymlinks(strings.TrimSpace(string(out)))
}

func NormalizeTargets(repo, cwd string, inputs []string) ([]string, error) {
	var targets []string
	for _, raw := range inputs {
		abs := raw
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(cwd, raw)
		}
		abs, e := filepath.Abs(abs)
		if e != nil {
			return nil, fmt.Errorf("resolve --target %q: %w", raw, e)
		}
		abs, e = filepath.EvalSymlinks(abs)
		if e != nil {
			return nil, fmt.Errorf("resolve --target %q: %w", raw, e)
		}
		rel, e := filepath.Rel(repo, abs)
		if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return nil, fmt.Errorf("--target %s: %s is outside the repository (%s)", raw, abs, repo)
		}
		if rel == "." {
			return nil, fmt.Errorf("--target %s: repository root is equivalent to no --target", raw)
		}
		info, e := os.Stat(abs)
		if e != nil {
			return nil, fmt.Errorf("--target %s: %s does not exist", raw, abs)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("--target %s: %s is not a directory", raw, abs)
		}
		targets = append(targets, filepath.ToSlash(rel))
	}
	slices.Sort(targets)
	targets = slices.Compact(targets)
	out := targets[:0]
	for _, t := range targets {
		under := false
		for _, other := range targets {
			if other != t && strings.HasPrefix(t, other+"/") {
				under = true
				break
			}
		}
		if !under {
			out = append(out, t)
		}
	}
	return out, nil
}

// Execute is the repo-facing command surface. Workflow callbacks own model and Git work.
func Execute(argv []string, cwd string, stdout, stderr io.Writer, callbacks Callbacks) int {
	args, err := Parse(argv)
	if err != nil {
		fmt.Fprintln(stderr, "refactor-me:", err)
		return ExitAborted
	}
	if args.Command == "help" && !args.Version {
		fmt.Fprint(stdout, Help)
		return ExitOK
	}
	if args.Version || args.Command == "version" {
		fmt.Fprint(stdout, VersionText(args.JSON))
		return ExitOK
	}
	if !slices.Contains([]string{"init", "run", "doctor", "report", "clean"}, args.Command) {
		fmt.Fprintln(stderr, "unknown command:", args.Command)
		fmt.Fprintln(stderr, "run `refactor-me help` for usage")
		return ExitAborted
	}
	repoDir := cwd
	if args.Repo != "" {
		repoDir = args.Repo
		if !filepath.IsAbs(repoDir) {
			repoDir = filepath.Join(cwd, repoDir)
		}
	}
	repo, err := RepoRoot(repoDir)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitAborted
	}
	if args.Command == "init" {
		if err := Init(repo); err != nil {
			fmt.Fprintln(stderr, "refactor-me:", err)
			return ExitAborted
		}
		fmt.Fprintln(stdout, "initialized refactor-me in", repo)
		return ExitOK
	}
	cfg, err := LoadConfig(repo)
	if err != nil {
		fmt.Fprintln(stderr, "refactor-me:", err)
		return ExitAborted
	}
	if args.Command == "report" {
		return executeReport(repo, args, stdout, stderr)
	}
	providers, err := ProviderOrder(cfg, args)
	if err != nil {
		fmt.Fprintln(stderr, "refactor-me:", err)
		return ExitAborted
	}
	cfg, settings, err := providerOptions(cfg, args, providers)
	if err != nil {
		fmt.Fprintln(stderr, "refactor-me:", err)
		return ExitAborted
	}
	targetDir := cwd
	if args.Repo != "" {
		targetDir = repo
	}
	targets, err := NormalizeTargets(repo, targetDir, args.Targets)
	if err != nil {
		fmt.Fprintln(stderr, "refactor-me:", err)
		return ExitAborted
	}
	cfg, limitSource, err := runDurationOptions(cfg, args, callbacks.SelectMaxMinutes)
	if err != nil {
		fmt.Fprintln(stderr, "refactor-me:", err)
		return ExitAborted
	}
	ctx := Context{Repo: repo, Config: cfg, Args: args, Providers: providers, ProviderSettings: settings, RunLimitSource: limitSource, Targets: targets, Stdout: stdout, Stderr: stderr}
	if args.Command == "clean" {
		if callbacks.Clean == nil {
			fmt.Fprintln(stderr, "clean is unavailable")
			return ExitAborted
		}
		out, e := callbacks.Clean(ctx)
		if e != nil {
			fmt.Fprintln(stderr, "refactor-me:", e)
			return ExitAborted
		}
		fmt.Fprint(stdout, out)
		return ExitOK
	}
	if args.Command == "run" {
		fmt.Fprintf(stderr, label(args.Language, "Starting refactoring in %s. Providers: %s", "%s 저장소에서 리팩토링을 시작합니다. provider: %s"), DisplayText(repo), DisplayText(strings.Join(providers, "+")))
	} else {
		fmt.Fprintf(stderr, "refactor-me  repo=%s  providers=%s", repo, strings.Join(providers, "+"))
	}
	if len(targets) > 0 {
		fmt.Fprintf(stderr, "  target=%s", DisplayText(strings.Join(targets, ",")))
	}
	fmt.Fprintln(stderr)
	if args.Command == "doctor" {
		if callbacks.Doctor == nil {
			fmt.Fprintln(stderr, "doctor is unavailable")
			return ExitAborted
		}
		result, e := callbacks.Doctor(ctx)
		if e != nil {
			fmt.Fprintln(stderr, "refactor-me:", e)
			return ExitAborted
		}
		if args.JSON && len(result.JSON) > 0 {
			stdout.Write(result.JSON)
		} else {
			fmt.Fprintln(stderr, result.Summary)
		}
		if result.OK {
			return ExitOK
		}
		return ExitAborted
	}
	if callbacks.Run == nil {
		fmt.Fprintln(stderr, "run is unavailable")
		return ExitAborted
	}
	result, e := callbacks.Run(ctx)
	if e != nil {
		fmt.Fprintln(stderr, "refactor-me:", e)
		return ExitAborted
	}
	if err := writeLastRun(repo, result); err != nil {
		fmt.Fprintln(stderr, "refactor-me: last-run:", err)
		return ExitUnsafe
	}
	if args.JSON {
		stdout.Write(result.Report)
	} else {
		fmt.Fprintln(stderr, result.Summary)
	}
	switch result.Status {
	case "HALTED_UNSAFE":
		return ExitUnsafe
	case "ABORTED":
		return ExitAborted
	default:
		return ExitOK
	}
}

func writeLastRun(repo string, r RunResult) error {
	if r.RunDir == "" {
		return errors.New("missing run directory")
	}
	ptr := map[string]any{"schemaVersion": LastRunSchemaVersion, "runId": r.RunID, "toolVersion": Version, "runDir": r.RunDir, "status": r.Status, "branch": r.Branch, "finishedAt": time.Now().UTC().Format(time.RFC3339Nano)}
	data, e := json.MarshalIndent(ptr, "", "  ")
	if e != nil {
		return e
	}
	data = append(data, '\n')
	dir := filepath.Join(repo, ".refactor")
	if e := checkDirectory(dir); e != nil {
		return e
	}
	if e := os.MkdirAll(dir, 0755); e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".last-run-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(data); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(f.Name(), filepath.Join(dir, "last-run.json"))
}
