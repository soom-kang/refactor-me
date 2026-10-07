package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/soom-kang/refactor-me/tool/go/internal/catalog"
	"github.com/soom-kang/refactor-me/tool/go/internal/workspace"
)

var Providers = []string{"claude", "codex"}
var writePhases = map[string]bool{"characterization": true, "execute": true}
var phaseEffort = map[string]string{"audit": "high", "deep_check": "high", "preflight": "medium", "characterization": "medium", "execute": "high", "review": "high", "handoff": "low", "doctor": "low"}

func ModeFor(phase string) string {
	if writePhases[phase] {
		return "write"
	}
	return "read"
}
func EffortFor(phase string, agent AgentConfig) string {
	if x := agent.EffortByPhase[phase]; x != "" {
		return x
	}
	if agent.Effort != "" {
		return agent.Effort
	}
	if x := phaseEffort[phase]; x != "" {
		return x
	}
	return "medium"
}

type AgentConfig struct {
	Bin           string            `json:"bin"`
	Model         string            `json:"model"`
	TimeoutSec    int               `json:"timeout_sec"`
	MaxBudgetUSD  float64           `json:"max_budget_usd"`
	Effort        string            `json:"effort"`
	EffortByPhase map[string]string `json:"effort_by_phase"`
}
type Config struct {
	Agents          map[string]AgentConfig `json:"agents"`
	Skills          *catalog.Catalog       `json:"-"`
	EffortOverrides map[string]string      `json:"-"`
}
type Request struct {
	Phase   string
	Mode    string
	CWD     string
	Body    string
	Schema  map[string]any
	RunDir  string
	Timeout time.Duration
	Attempt string
	Effort  string
}
type Usage struct {
	InputTokens             int64                 `json:"inputTokens"`
	OutputTokens            int64                 `json:"outputTokens"`
	UncachedInputTokens     int64                 `json:"uncachedInputTokens,omitempty"`
	CachedInputTokens       int64                 `json:"cachedInputTokens,omitempty"`
	CacheWriteInputTokens   int64                 `json:"cacheWriteInputTokens,omitempty"`
	CacheWrite1hInputTokens int64                 `json:"cacheWrite1hInputTokens,omitempty"`
	ReasoningOutputTokens   int64                 `json:"reasoningOutputTokens,omitempty"`
	CostUSD                 *float64              `json:"costUsd"`
	CostMissing             int                   `json:"costMissing,omitempty"`
	EstimatedCostUSD        *float64              `json:"estimatedCostUsd,omitempty"`
	EstimateMissing         int                   `json:"estimateMissing,omitempty"`
	CostEstimates           []CostEstimate        `json:"costEstimates,omitempty"`
	EstimateMissingReasons  []CostEstimateMissing `json:"estimateMissingReasons,omitempty"`
	usageReported           bool
	invalidUsage            bool
	cacheReadReported       bool
	cacheWriteReported      bool
	cacheWriteTTLReported   bool
	invalidReportedCost     bool
}
type Result struct {
	OK                bool   `json:"ok"`
	Failure           string `json:"failure"`
	Hard              bool   `json:"hard"`
	Fatal             bool   `json:"fatal"`
	Detail            string `json:"detail,omitempty"`
	Data              any    `json:"data"`
	Text              string `json:"text"`
	Provider          string `json:"provider"`
	SessionID         string `json:"sessionId,omitempty"`
	ExitCode          *int   `json:"exitCode"`
	Usage             Usage  `json:"usage"`
	RawPath           string `json:"rawPath,omitempty"`
	ToolCalls         int    `json:"toolCalls"`
	DurationMS        int64  `json:"durationMs"`
	Processes         int    `json:"processes"`
	PermissionDenials int    `json:"-"`
}

// ProviderEvent carries only fixed event metadata, never tool arguments or output.
type ProviderEvent struct {
	Kind       string
	Provider   string
	Phase      string
	Count      int
	Action     string
	Status     string
	Path       string
	DurationMS *int64
	ToolCalls  *int
	ExitCode   *int
}

// Logger receives provider state transitions, never prompt or environment values.
type Logger interface {
	OnProviderEvent(ProviderEvent)
}

func ClaudeArgs(mode string, schema map[string]any, model, effort string, budget float64) []string {
	a := []string{"-p", "--output-format", "stream-json", "--verbose"}
	if schema != nil {
		raw, _ := json.Marshal(schema)
		a = append(a, "--json-schema", string(raw))
	}
	if mode == "write" {
		a = append(a, "--permission-mode", "acceptEdits", "--tools", "Skill,Read,Glob,Grep,Edit,Write", "--disallowed-tools", "Bash", "NotebookEdit", "WebFetch", "WebSearch")
	} else {
		a = append(a, "--permission-mode", "plan", "--tools", "Skill,Read,Glob,Grep", "--disallowed-tools", "Edit", "Write", "NotebookEdit", "Bash", "WebFetch", "WebSearch")
	}
	a = append(a, "--setting-sources", "project", "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--no-session-persistence", "--no-chrome")
	if model != "" {
		a = append(a, "--model", model)
	}
	if effort != "" {
		a = append(a, "--effort", effort)
	}
	if budget > 0 {
		a = append(a, "--max-budget-usd", strconv.FormatFloat(budget, 'f', -1, 64))
	}
	return a
}
func CodexArgs(mode, cwd, schemaPath, outPath, model, effort string) []string {
	if effort == "" {
		effort = "high"
	}
	sandbox := "read-only"
	if mode == "write" {
		sandbox = "workspace-write"
	}
	a := []string{"exec", "-", "--sandbox", sandbox, "--cd", cwd}
	if schemaPath != "" {
		a = append(a, "--output-schema", schemaPath)
	}
	a = append(a, "--output-last-message", outPath, "--json", "--ignore-user-config", "--ignore-rules")
	if model != "" {
		a = append(a, "-c", fmt.Sprintf("model=%q", model))
	}
	return append(a, "-c", fmt.Sprintf("model_reasoning_effort=%q", effort))
}

var envAllowlist = []string{"PATH", "HOME", "USER", "LOGNAME", "SHELL", "TMPDIR", "LANG", "LC_ALL", "TERM", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL", "CLAUDE_CONFIG_DIR", "OPENAI_API_KEY", "OPENAI_BASE_URL", "CODEX_HOME", "NVM_DIR", "NODE_PATH", "GOPATH", "GOROOT", "GOMODCACHE", "CARGO_HOME", "RUSTUP_HOME", "PYENV_ROOT", "VIRTUAL_ENV", "JAVA_HOME", "BUN_INSTALL", "PNPM_HOME"}

func SanitizedEnv() []string {
	out := make([]string, 0, len(envAllowlist)+3)
	for _, k := range envAllowlist {
		if v, ok := os.LookupEnv(k); ok {
			out = append(out, k+"="+v)
		}
	}
	return append(out, "FORCE_COLOR=0", "NO_COLOR=1", "CI=1")
}

var processKillGrace = 10 * time.Second

type processResult struct {
	stdout, stderr string
	exitCode       *int
	killed         bool
}
type eventWriter struct {
	output  bytes.Buffer
	pending string
	onEvent func(map[string]any)
}

func (w *eventWriter) Write(p []byte) (int, error) {
	n, err := w.output.Write(p)
	if w.onEvent != nil {
		w.pending += string(p)
		for {
			i := strings.IndexByte(w.pending, '\n')
			if i < 0 {
				break
			}
			line := strings.TrimSpace(w.pending[:i])
			w.pending = w.pending[i+1:]
			var ev map[string]any
			if json.Unmarshal([]byte(line), &ev) == nil {
				w.onEvent(ev)
			}
		}
	}
	return n, err
}
func runProcess(ctx context.Context, bin string, args []string, req Request, onEvent func(map[string]any)) processResult {
	var r processResult
	if ctx.Err() != nil {
		r.killed = true
		return r
	}
	// CommandContext's default Cancel only kills the direct child. Manage the
	// process group ourselves so descendants cannot outlive a timed-out call.
	cmd := exec.Command(bin, args...)
	cmd.Dir = req.CWD
	cmd.Env = SanitizedEnv()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdin = strings.NewReader(req.Body)
	stdout := &eventWriter{onEvent: onEvent}
	var stderr bytes.Buffer
	cmd.Stdout = stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		r.stderr = err.Error()
		return r
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	var waitErr error
	select {
	case waitErr = <-wait:
	case <-timer.C:
		r.killed = true
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		select {
		case waitErr = <-wait:
			// The leader may exit before a TERM-resistant descendant.
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		case <-time.After(processKillGrace):
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			waitErr = <-wait
		}
	case <-ctx.Done():
		r.killed = true
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		select {
		case waitErr = <-wait:
			// The leader may exit before a TERM-resistant descendant.
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		case <-time.After(processKillGrace):
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			waitErr = <-wait
		}
	}
	r.stdout = stdout.output.String()
	r.stderr = stderr.String()
	if !r.killed {
		code := 0
		if waitErr != nil {
			if ee := new(exec.ExitError); errors.As(waitErr, &ee) {
				code = ee.ExitCode()
			} else {
				code = -1
				if r.stderr != "" {
					r.stderr += "\n"
				}
				r.stderr += waitErr.Error()
			}
		}
		r.exitCode = &code
	}
	return r
}

func StripFences(s string) string {
	s = strings.TrimSpace(s)
	re := regexp.MustCompile("(?s)^```(?:json)?\\s*\\n(.*?)\\n?```$")
	if m := re.FindStringSubmatch(s); m != nil {
		s = m[1]
	}
	return strings.TrimSpace(s)
}
func LastJSONObject(s string) map[string]any {
	depth, start, best := 0, -1, ""
	inString, escape := false, false
	for i, c := range s {
		if inString {
			if escape {
				escape = false
			} else if c == '\\' {
				escape = true
			} else if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			continue
		}
		if c == '{' {
			if depth == 0 {
				start = i
			}
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 && start >= 0 {
				best = s[start : i+1]
			}
		}
	}
	var v map[string]any
	if json.Unmarshal([]byte(best), &v) != nil {
		return nil
	}
	return v
}

var quotaHard = regexp.MustCompile(`(?i)usage limit reached|hit your usage limit|credit balance (is )?too low|out of usage credits|monthly spend limit|insufficient_quota|upgrade to increase your usage limit|weekly limit|resource_exhausted`)
var auth = regexp.MustCompile(`(?i)failed to authenticate|oauth session expired|invalid api key|invalid_api_key|please run /login|codex login|not logged in|authentication_error|unauthorized|\b401\b|missing (openai_api_key|api key)`)
var quotaSoft = regexp.MustCompile(`(?i)rate.?limit|too many requests|overloaded|\b429\b|\b529\b`)
var missingCLI = regexp.MustCompile(`(?i)ENOENT|command not found|No such file`)
var schemaReject = regexp.MustCompile(`(?i)output[- ]schema|invalid schema|schema file|failed to parse schema`)

func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if x := strings.TrimSpace(l); x != "" {
			return x
		}
	}
	return ""
}

type classification struct {
	failure, detail string
	hard, fatal     bool
	parsed          Result
}

func parseClaude(stdout string) Result {
	var env map[string]any
	for _, line := range strings.Split(stdout, "\n") {
		var ev map[string]any
		if json.Unmarshal([]byte(line), &ev) == nil && ev["type"] == "result" {
			env = ev
		}
	}
	if env == nil {
		env = LastJSONObject(stdout)
	}
	if env == nil {
		return Result{}
	}
	var r Result
	r.Text, _ = env["result"].(string)
	r.Data = env["structured_output"]
	r.SessionID, _ = env["session_id"].(string)
	if usage, ok := env["usage"].(map[string]any); ok {
		r.Usage = parseTokenUsage(usage, "claude")
	}
	if value, present := env["total_cost_usd"]; present && value != nil {
		if cost, ok := value.(float64); ok && cost >= 0 && !math.IsNaN(cost) && !math.IsInf(cost, 0) {
			r.Usage.CostUSD = &cost
		} else {
			r.Usage.invalidReportedCost = true
		}
	}
	if d, ok := env["permission_denials"].([]any); ok {
		r.PermissionDenials = len(d)
	}
	return r
}
func parseCodex(stdout, outPath string) Result {
	var r Result
	if b, err := os.ReadFile(outPath); err == nil {
		r.Text = string(b)
		_ = json.Unmarshal([]byte(StripFences(r.Text)), &r.Data)
	}
	for _, line := range strings.Split(stdout, "\n") {
		var ev map[string]any
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		if ev["type"] == "thread.started" {
			if id, ok := ev["thread_id"].(string); ok {
				r.SessionID = id
			}
		}
		if ev["type"] == "turn.completed" {
			if u, ok := ev["usage"].(map[string]any); ok {
				// Codex reports the session total; the last completed turn replaces it.
				r.Usage = parseTokenUsage(u, "codex")
			}
		}
	}
	return r
}
func classifyClaude(res processResult, requested bool) classification {
	p := parseClaude(res.stdout)
	if res.killed {
		return classification{failure: "TIMEOUT", detail: "killed after timeout", parsed: p}
	}
	env := LastJSONObject(res.stdout)
	for _, line := range strings.Split(res.stdout, "\n") {
		var ev map[string]any
		if json.Unmarshal([]byte(line), &ev) == nil && ev["type"] == "result" {
			env = ev
		}
	}
	if env == nil {
		if missingCLI.MatchString(res.stderr) {
			return classification{failure: "PROCESS", detail: "cli_missing", fatal: true}
		}
		return classification{failure: "PROCESS", detail: "unparseable stdout"}
	}
	msg := ""
	if value := env["result"]; value != nil {
		msg = fmt.Sprint(value)
	}
	msg += " " + res.stderr
	if env["subtype"] == "error_max_structured_output_retries" || env["terminal_reason"] == "structured_output_retry_exhausted" {
		return classification{failure: "SCHEMA", detail: "provider retries exhausted", parsed: p}
	}
	if env["is_error"] == true {
		if quotaHard.MatchString(msg) {
			return classification{failure: "QUOTA", hard: true, detail: firstLine(msg), parsed: p}
		}
		if auth.MatchString(msg) || env["api_error_status"] == float64(401) {
			return classification{failure: "AUTH", detail: firstLine(msg), parsed: p}
		}
		if quotaSoft.MatchString(msg) || env["api_error_status"] == float64(429) || env["api_error_status"] == float64(529) {
			return classification{failure: "QUOTA", detail: firstLine(msg), parsed: p}
		}
		detail := fmt.Sprint(env["terminal_reason"])
		if env["terminal_reason"] == nil {
			detail = firstLine(msg)
		}
		if detail == "" {
			detail = "unknown"
		}
		return classification{failure: "PROCESS", detail: detail, parsed: p}
	}
	if requested && p.Data == nil {
		return classification{failure: "SCHEMA", detail: "no structured_output on a successful turn", parsed: p}
	}
	return classification{failure: "OK", parsed: p}
}
func classifyCodex(res processResult, outPath string, requested bool) classification {
	p := parseCodex(res.stdout, outPath)
	if res.killed {
		return classification{failure: "TIMEOUT", detail: "killed after timeout", parsed: p}
	}
	msg := res.stderr + "\n" + res.stdout
	if res.exitCode == nil || *res.exitCode != 0 {
		if schemaReject.MatchString(res.stderr) {
			return classification{failure: "PROCESS", detail: "schema file rejected by codex", fatal: true, parsed: p}
		}
		if quotaHard.MatchString(msg) {
			return classification{failure: "QUOTA", hard: true, detail: firstLine(res.stderr), parsed: p}
		}
		if auth.MatchString(msg) {
			return classification{failure: "AUTH", detail: firstLine(res.stderr), parsed: p}
		}
		if quotaSoft.MatchString(msg) {
			return classification{failure: "QUOTA", detail: firstLine(res.stderr), parsed: p}
		}
		if missingCLI.MatchString(msg) {
			return classification{failure: "PROCESS", detail: "cli_missing", fatal: true, parsed: p}
		}
		detail := firstLine(res.stderr)
		if detail == "" {
			detail = "exit -1"
			if res.exitCode != nil {
				detail = fmt.Sprintf("exit %d", *res.exitCode)
			}
		}
		return classification{failure: "PROCESS", detail: detail, parsed: p}
	}
	if requested {
		if strings.TrimSpace(p.Text) == "" {
			return classification{failure: "SCHEMA", detail: "empty last message", parsed: p}
		}
		if p.Data == nil {
			return classification{failure: "SCHEMA", detail: "last message is not valid JSON", parsed: p}
		}
	}
	return classification{failure: "OK", parsed: p}
}

// recognizedToolEvent preserves the tool-call accounting boundary: one call for
// each Claude assistant event containing a tool use, or completed Codex command
// or file-change item. It does not extract data intended for terminal output.
func recognizedToolEvent(ev map[string]any) bool {
	if ev["type"] == "assistant" {
		if m, ok := ev["message"].(map[string]any); ok {
			if parts, ok := m["content"].([]any); ok {
				for _, x := range parts {
					c, ok := x.(map[string]any)
					if !ok || c["type"] != "tool_use" {
						continue
					}
					return true
				}
			}
		}
	}
	if ev["type"] == "item.completed" {
		if item, ok := ev["item"].(map[string]any); ok {
			if item["type"] == "command_execution" || item["type"] == "file_change" {
				return true
			}
		}
	}
	return false
}
func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// CallProvider performs one isolated CLI invocation and stores its raw transcript
// under req.RunDir. It never resumes a provider session.
func CallProvider(ctx context.Context, provider string, req Request, cfg Config, log Logger) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if provider != "claude" && provider != "codex" {
		return Result{}, fmt.Errorf("unknown provider %q", provider)
	}
	mode := req.Mode
	if mode == "" {
		mode = "read"
	}
	if mode != "read" && mode != "write" {
		return Result{}, fmt.Errorf("invalid provider mode: %s", mode)
	}
	if mode == "write" && !writePhases[req.Phase] {
		return Result{}, fmt.Errorf("phase %q requested write mode but is not in WRITE_PHASES", req.Phase)
	}
	if req.CWD == "" || req.RunDir == "" {
		return Result{}, errors.New("provider cwd and run directory are required")
	}
	if cfg.Skills != nil {
		if err := cfg.Skills.Verify(); err != nil {
			return Result{}, fmt.Errorf("%w: %v", workspace.ErrUnsafe, err)
		}
		if err := cfg.Skills.CheckWorkspace(req.CWD); err != nil {
			return Result{}, fmt.Errorf("%w: %v", workspace.ErrUnsafe, err)
		}
		if provider == "claude" && cfg.Skills.ClaudeRoot == "" {
			return Result{}, fmt.Errorf("%w: missing claude skill snapshot", workspace.ErrUnsafe)
		}
		req.Body += cfg.Skills.Prompt(provider)
	}
	agent := cfg.Agents[provider]
	bin := agent.Bin
	if bin == "" {
		bin = provider
	}
	if req.Timeout <= 0 {
		seconds := agent.TimeoutSec
		if seconds <= 0 {
			seconds = 1800
		}
		req.Timeout = time.Duration(seconds) * time.Second
	}
	effort := cfg.EffortOverrides[provider]
	if effort == "" {
		effort = req.Effort
	}
	if effort == "" {
		effort = EffortFor(req.Phase, agent)
	}
	attempt := req.Attempt
	if attempt == "" {
		attempt = "primary"
	}
	var args []string
	outPath := ""
	if provider == "claude" {
		args = ClaudeArgs(mode, req.Schema, agent.Model, effort, agent.MaxBudgetUSD)
		if cfg.Skills != nil {
			args = append(args, "--add-dir", cfg.Skills.ClaudeRoot)
		}
	} else {
		outPath = filepath.Join(req.RunDir, "provider", req.Phase+"-last.json")
		if err := os.MkdirAll(filepath.Dir(outPath), 0700); err != nil {
			return Result{}, fmt.Errorf("create provider directory: %w", err)
		}
		if err := os.Remove(outPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Result{}, fmt.Errorf("remove stale provider output: %w", err)
		}
		schemaPath := ""
		if req.Schema != nil {
			schemaPath = filepath.Join(req.RunDir, "schemas", req.Phase+".json")
			if err := os.MkdirAll(filepath.Dir(schemaPath), 0700); err != nil {
				return Result{}, fmt.Errorf("create schema directory: %w", err)
			}
			raw, err := json.MarshalIndent(req.Schema, "", "  ")
			if err != nil {
				return Result{}, fmt.Errorf("encode provider schema: %w", err)
			}
			if err := os.WriteFile(schemaPath, raw, 0600); err != nil {
				return Result{}, fmt.Errorf("write provider schema: %w", err)
			}
		}
		args = CodexArgs(mode, req.CWD, schemaPath, outPath, agent.Model, effort)
	}
	tools := 0
	started := time.Now()
	if log != nil {
		log.OnProviderEvent(ProviderEvent{Kind: "activity", Provider: provider, Phase: req.Phase, Action: "provider", Status: "started"})
	}
	proc := runProcess(ctx, bin, args, req, func(ev map[string]any) {
		if recognizedToolEvent(ev) {
			tools++
		}
		if log != nil {
			for _, activity := range ActivityEvents(provider, req.Phase, req.CWD, ev) {
				log.OnProviderEvent(activity)
			}
		}
	})
	duration := time.Since(started).Milliseconds()
	var unsafeErr error
	if cfg.Skills != nil {
		if err := cfg.Skills.Verify(); err != nil {
			unsafeErr = fmt.Errorf("%w: %v", workspace.ErrUnsafe, err)
		}
		if err := cfg.Skills.CheckWorkspace(req.CWD); err != nil {
			unsafeErr = errors.Join(unsafeErr, fmt.Errorf("%w: %v", workspace.ErrUnsafe, err))
		}
	}
	rawBase := filepath.Join(req.RunDir, "provider", fmt.Sprintf("%s-%s-%s", req.Phase, provider, attempt))
	var cls classification
	if provider == "claude" {
		cls = classifyClaude(proc, req.Schema != nil)
	} else {
		cls = classifyCodex(proc, outPath, req.Schema != nil)
	}
	p := cls.parsed
	if mode == "read" && p.PermissionDenials > 0 && log != nil {
		log.OnProviderEvent(ProviderEvent{Kind: "permission_denied", Provider: provider, Phase: req.Phase, Count: p.PermissionDenials})
	}
	p.OK = cls.failure == "OK"
	p.Failure = cls.failure
	p.Hard = cls.hard
	p.Fatal = cls.fatal
	p.Detail = cls.detail
	p.Provider = provider
	p.ExitCode = proc.exitCode
	p.RawPath = rawBase + ".stdout.jsonl"
	p.ToolCalls = tools
	p.DurationMS = duration
	p.Processes = 1
	p.Usage = EstimateUsage(p.Usage, provider, agent.Model, req.Phase)
	if unsafeErr != nil {
		p.OK = false
		p.Failure = "UNSAFE"
		p.Fatal = true
		p.Detail = unsafeErr.Error()
	}
	// Account for the completed invocation before storing transcripts. A storage
	// failure must preserve observed usage and any independent safety failure.
	writeTranscripts := func() error {
		if err := os.MkdirAll(filepath.Dir(rawBase), 0700); err != nil {
			return fmt.Errorf("create transcript directory: %w", err)
		}
		if err := os.WriteFile(rawBase+".prompt.md", []byte(req.Body), 0600); err != nil {
			return fmt.Errorf("write prompt transcript: %w", err)
		}
		if err := os.WriteFile(rawBase+".stdout.jsonl", []byte(proc.stdout), 0600); err != nil {
			return fmt.Errorf("write stdout transcript: %w", err)
		}
		if strings.TrimSpace(proc.stderr) != "" {
			if err := os.WriteFile(rawBase+".stderr.log", []byte(proc.stderr), 0600); err != nil {
				return fmt.Errorf("write stderr transcript: %w", err)
			}
		}
		return nil
	}
	callErr := unsafeErr
	if err := writeTranscripts(); err != nil {
		callErr = errors.Join(callErr, err)
		p.OK = false
		if unsafeErr == nil {
			p.Failure = "PROCESS"
		}
		p.Detail = callErr.Error()
	}
	if log != nil {
		status := "completed"
		if !p.OK {
			status = "failed"
		}
		log.OnProviderEvent(ProviderEvent{Kind: "activity", Provider: provider, Phase: req.Phase, Action: "provider", Status: status,
			DurationMS: &p.DurationMS, ToolCalls: &p.ToolCalls, ExitCode: p.ExitCode})
	}
	return p, callErr
}

// CallWithRepair checks the schema and makes at most one read-only repair call.
// The repair can format an existing result but cannot repeat a write phase.
func CallWithRepair(ctx context.Context, provider string, req Request, cfg Config, log Logger) (Result, error) {
	res, err := CallProvider(ctx, provider, req, cfg, log)
	if err != nil {
		return res, err
	}
	if err := ctx.Err(); err != nil {
		return res, err
	}
	if res.OK && req.Schema != nil {
		if errs := Validate(req.Schema, res.Data); len(errs) == 0 {
			return res, nil
		} else {
			res.OK = false
			res.Failure = "SCHEMA"
			res.Detail = strings.Join(errs[:min(5, len(errs))], "; ")
		}
	}
	if res.Failure != "SCHEMA" || res.Fatal {
		return res, nil
	}
	if log != nil {
		log.OnProviderEvent(ProviderEvent{Kind: "schema_repair", Provider: provider, Phase: req.Phase, Count: 1})
	}
	req.Mode = "read"
	req.Attempt = "repair"
	req.Body = RepairPrompt(NewNonce(), res.Detail, res.Text)
	repair, err := CallProvider(ctx, provider, req, cfg, log)
	if err != nil {
		repair.Usage = MergeUsage(res.Usage, repair.Usage)
		repair.DurationMS += res.DurationMS
		repair.Processes += res.Processes
		return repair, err
	}
	if repair.OK && req.Schema != nil {
		if errs := Validate(req.Schema, repair.Data); len(errs) > 0 {
			repair.OK = false
			repair.Failure = "SCHEMA"
			repair.Detail = strings.Join(errs[:min(5, len(errs))], "; ")
		}
	}
	repair.Usage = MergeUsage(res.Usage, repair.Usage)
	repair.DurationMS += res.DurationMS
	repair.Processes += res.Processes
	return repair, nil
}
func MergeUsage(a, b Usage) Usage {
	u := Usage{InputTokens: a.InputTokens + b.InputTokens, OutputTokens: a.OutputTokens + b.OutputTokens,
		UncachedInputTokens: a.UncachedInputTokens + b.UncachedInputTokens, CachedInputTokens: a.CachedInputTokens + b.CachedInputTokens,
		CacheWriteInputTokens: a.CacheWriteInputTokens + b.CacheWriteInputTokens, CacheWrite1hInputTokens: a.CacheWrite1hInputTokens + b.CacheWrite1hInputTokens,
		ReasoningOutputTokens: a.ReasoningOutputTokens + b.ReasoningOutputTokens, CostUSD: sumCost(a.CostUSD, b.CostUSD), CostMissing: a.CostMissing + b.CostMissing,
		EstimatedCostUSD: sumCost(a.EstimatedCostUSD, b.EstimatedCostUSD), EstimateMissing: a.EstimateMissing + b.EstimateMissing,
		CostEstimates:          append(append([]CostEstimate{}, a.CostEstimates...), b.CostEstimates...),
		EstimateMissingReasons: append(append([]CostEstimateMissing{}, a.EstimateMissingReasons...), b.EstimateMissingReasons...)}
	if a.CostUSD == nil && a.CostMissing == 0 {
		u.CostMissing++
	}
	if b.CostUSD == nil && b.CostMissing == 0 {
		u.CostMissing++
	}
	return u
}

// ProviderVersion reads only local CLI version metadata with a bounded subprocess.
func ProviderVersion(ctx context.Context, bin, cwd string) (string, error) {
	result := runProcess(ctx, bin, []string{"--version"}, Request{CWD: cwd, Timeout: 10 * time.Second}, nil)
	if result.killed || result.exitCode == nil || *result.exitCode != 0 {
		return "", errors.New("provider version command failed")
	}
	line := firstLine(result.stdout)
	if line == "" {
		return "", errors.New("provider version command returned empty output")
	}
	return truncate(line, 256), nil
}
