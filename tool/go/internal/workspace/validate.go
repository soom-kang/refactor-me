package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"
)

const (
	TierFast         = "T1"
	TierTest         = "T2"
	TierBuild        = "T3"
	StatusGreen      = "GREEN"
	StatusRed        = "RED"
	StatusTimeout    = "TIMEOUT"
	StatusUnrunnable = "UNRUNNABLE"
	StatusOpaque     = "OPAQUE"
)

var tierTimeout = map[string]time.Duration{TierFast: 5 * time.Minute, TierTest: 15 * time.Minute, TierBuild: 15 * time.Minute}
var heavyRE = regexp.MustCompile(`(?i)playwright|cypress|--watch\b|\be2e\b|vitest\s+--ui|testcontainers|docker|compose|psql|migrate|-race\b`)
var prune = map[string]bool{"node_modules": true, "vendor": true, ".git": true, "dist": true, "build": true, ".next": true, "target": true, ".venv": true, "venv": true, "coverage": true, ".refactor": true, ".agents": true, ".claude": true}
var markers = []string{"package.json", "go.mod", "pyproject.toml", "Cargo.toml", "Gemfile", "Makefile"}
var jvmMarkers = []string{"settings.gradle", "settings.gradle.kts", "build.gradle", "build.gradle.kts", "pom.xml"}

type Command struct {
	ID        string   `json:"id"`
	Area      string   `json:"area"`
	Tier      string   `json:"tier"`
	Family    string   `json:"family"`
	Name      string   `json:"name"`
	Argv      []string `json:"argv"`
	Cwd       string   `json:"cwd"`
	TimeoutMS int      `json:"timeoutMs"`
	Source    string   `json:"source"`
	Reason    string   `json:"reason,omitempty"`
}
type CIHint struct {
	File    string `json:"file"`
	Command string `json:"command"`
}
type Discovery struct {
	Areas    []string  `json:"areas"`
	Commands []Command `json:"commands"`
	Skipped  []Command `json:"skipped"`
	Hints    []CIHint  `json:"hints"`
	Locked   bool      `json:"locked"`
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }
func hasAny(dir string, names []string) bool {
	for _, n := range names {
		if exists(filepath.Join(dir, n)) {
			return true
		}
	}
	return false
}
func ancestorMarker(root, dir string, names []string) bool {
	if filepath.Clean(root) == filepath.Clean(dir) {
		return false
	}
	for cur := filepath.Dir(dir); ; cur = filepath.Dir(cur) {
		if hasAny(cur, names) {
			return true
		}
		if cur == root || filepath.Dir(cur) == cur {
			return false
		}
	}
}

func DetectAreas(root string, maxDepth int) []string {
	found := map[string]bool{".": true}
	var walk func(string, int)
	walk = func(dir string, depth int) {
		if depth > maxDepth {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if !e.IsDir() || prune[e.Name()] || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			abs := filepath.Join(dir, e.Name())
			rel, _ := filepath.Rel(root, abs)
			if hasAny(abs, markers) || (hasAny(abs, jvmMarkers) && !ancestorMarker(root, abs, jvmMarkers)) {
				found[filepath.ToSlash(rel)] = true
			}
			walk(abs, depth+1)
		}
	}
	walk(root, 1)
	areas := make([]string, 0, len(found))
	for a := range found {
		areas = append(areas, a)
	}
	slices.Sort(areas)
	return areas
}

func addCommand(out *[]Command, area, family, tier, name string, argv []string, source string) {
	if len(argv) == 0 {
		return
	}
	id := area + ":" + tier + ":" + family + ":" + name
	for _, c := range *out {
		if c.ID == id {
			return
		}
	}
	*out = append(*out, Command{ID: id, Area: area, Tier: tier, Family: family, Name: name, Argv: argv, Cwd: area, TimeoutMS: int(tierTimeout[tier] / time.Millisecond), Source: source})
}
func packageRunner(dir string) string {
	if hasAny(dir, []string{"bun.lock", "bun.lockb"}) {
		return "bun"
	}
	if exists(filepath.Join(dir, "pnpm-lock.yaml")) {
		return "pnpm"
	}
	if exists(filepath.Join(dir, "yarn.lock")) {
		return "yarn"
	}
	return "npm"
}
func pickScript(scripts map[string]any, names ...string) (string, string) {
	for _, name := range names {
		if value, ok := scripts[name].(string); ok {
			return name, value
		}
	}
	return "", ""
}
func wrapper(dir, name, fallback string) string {
	info, err := os.Stat(filepath.Join(dir, name))
	if err == nil && info.Mode()&0o111 != 0 {
		return "./" + name
	}
	return fallback
}
func pyRunner(dir string) []string {
	if exists(filepath.Join(dir, "uv.lock")) {
		return []string{"uv", "run", "--"}
	}
	if exists(filepath.Join(dir, "poetry.lock")) {
		return []string{"poetry", "run"}
	}
	return nil
}

var jvmPluginRE = regexp.MustCompile(`(?m)\bid\s*[\("']\s*(?:java|java-library|application|groovy|scala)\b|\bkotlin\s*\(\s*["']jvm|org\.jetbrains\.kotlin\.jvm|apply\s+plugin:\s*['"](?:java|java-library|application|groovy|scala|kotlin)['"]`)

func gradleScripts(dir string) string {
	var b strings.Builder
	var read = func(d string) {
		for _, n := range jvmMarkers {
			v, err := os.ReadFile(filepath.Join(d, n))
			if err == nil {
				b.Write(v)
				b.WriteByte('\n')
			}
		}
	}
	read(dir)
	entries, _ := os.ReadDir(dir)
	seen := 0
	for _, e := range entries {
		if seen >= 50 {
			break
		}
		if e.IsDir() && !prune[e.Name()] && !strings.HasPrefix(e.Name(), ".") {
			read(filepath.Join(dir, e.Name()))
			seen++
		}
	}
	return b.String()
}

func DiscoverArea(root, area string) []Command {
	dir := filepath.Join(root, area)
	var out []Command
	if b, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil {
		var pkg struct {
			Scripts map[string]any `json:"scripts"`
		}
		if json.Unmarshal(b, &pkg) == nil && pkg.Scripts != nil {
			runner := packageRunner(dir)
			if n, _ := pickScript(pkg.Scripts, "typecheck", "type-check", "tsc"); n != "" {
				addCommand(&out, area, "npm", TierFast, "typecheck", []string{runner, "run", n}, "package.json scripts."+n)
			}
			if n, s := pickScript(pkg.Scripts, "lint"); n != "" && !heavyRE.MatchString(s) {
				addCommand(&out, area, "npm", TierFast, "lint", []string{runner, "run", n}, "package.json scripts."+n)
			}
			if n, s := pickScript(pkg.Scripts, "test"); n != "" {
				if heavyRE.MatchString(s) {
					out = append(out, Command{ID: area + ":SKIPPED:npm:test", Area: area, Tier: "SKIPPED", Family: "npm", Name: "test", Argv: []string{runner, "run", n}, Cwd: area, Source: "package.json scripts." + n, Reason: "looks like an e2e/browser/service suite"})
				} else {
					addCommand(&out, area, "npm", TierTest, "test", []string{runner, "run", n}, "package.json scripts."+n)
				}
			}
			if n, s := pickScript(pkg.Scripts, "build"); n != "" && !heavyRE.MatchString(s) {
				addCommand(&out, area, "npm", TierBuild, "build", []string{runner, "run", n}, "package.json scripts."+n)
			}
		}
	}
	if exists(filepath.Join(dir, "go.mod")) {
		addCommand(&out, area, "go", TierFast, "vet", []string{"go", "vet", "./..."}, "go.mod")
		addCommand(&out, area, "go", TierTest, "test", []string{"go", "test", "-count=1", "./..."}, "go.mod")
		addCommand(&out, area, "go", TierBuild, "build", []string{"go", "build", "./..."}, "go.mod")
	}
	if b, err := os.ReadFile(filepath.Join(dir, "pyproject.toml")); err == nil {
		body := string(b)
		for _, spec := range []struct {
			tool, tier, name string
			args             []string
		}{{"ruff", TierFast, "lint", []string{"ruff", "check", "."}}, {"mypy", TierFast, "typecheck", []string{"mypy", "."}}, {"pytest", TierTest, "test", []string{"pytest", "-q", "-m", "not integration"}}} {
			if !strings.Contains(body, spec.tool) {
				continue
			}
			args := spec.args
			if regexp.MustCompile(`["']` + spec.tool + `\b`).MatchString(body) {
				args = append(pyRunner(dir), args...)
			}
			addCommand(&out, area, "py", spec.tier, spec.name, args, "pyproject.toml "+spec.tool)
		}
	}
	if exists(filepath.Join(dir, "Cargo.toml")) {
		addCommand(&out, area, "rust", TierFast, "check", []string{"cargo", "check"}, "Cargo.toml")
		addCommand(&out, area, "rust", TierTest, "test", []string{"cargo", "test", "--lib"}, "Cargo.toml")
	}
	if hasAny(dir, []string{"build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts"}) && !ancestorMarker(root, dir, jvmMarkers) && jvmPluginRE.MatchString(gradleScripts(dir)) {
		g := wrapper(dir, "gradlew", "gradle")
		addCommand(&out, area, "gradle", TierFast, "typecheck", []string{g, "testClasses", "--console=plain"}, "build.gradle (JVM plugin)")
		addCommand(&out, area, "gradle", TierTest, "test", []string{g, "test", "--console=plain"}, "build.gradle (JVM plugin)")
	}
	if exists(filepath.Join(dir, "pom.xml")) && !ancestorMarker(root, dir, jvmMarkers) {
		m := wrapper(dir, "mvnw", "mvn")
		addCommand(&out, area, "maven", TierFast, "typecheck", []string{m, "-B", "test-compile"}, "pom.xml")
		addCommand(&out, area, "maven", TierTest, "test", []string{m, "-B", "test"}, "pom.xml")
	}
	if b, err := os.ReadFile(filepath.Join(dir, "Makefile")); err == nil {
		taken := map[string]bool{}
		for _, c := range out {
			taken[c.Name] = true
		}
		body := string(b)
		for _, name := range []string{"lint", "typecheck", "check", "test", "build"} {
			if taken[name] {
				continue
			}
			re := regexp.MustCompile(`(?m)^` + name + `:`)
			loc := re.FindStringIndex(body)
			if loc == nil {
				continue
			}
			rest := body[loc[0]:]
			lines := strings.Split(rest, "\n")
			end := len(lines)
			for i := 1; i < len(lines); i++ {
				if lines[i] != "" && lines[i][0] != '\t' && lines[i][0] != ' ' {
					end = i
					break
				}
			}
			rest = strings.Join(lines[:end], "\n")
			if heavyRE.MatchString(rest) {
				continue
			}
			tier := TierFast
			if name == "test" {
				tier = TierTest
			}
			if name == "build" {
				tier = TierBuild
			}
			addCommand(&out, area, "make", tier, name, []string{"make", name}, "Makefile target "+name)
		}
	}
	return out
}

func CIHints(root string) []CIHint {
	dir := filepath.Join(root, ".github", "workflows")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var hints []CIHint
	re := regexp.MustCompile(`^\s*(?:-\s*)?run:\s*(.+)$`)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".yml") && !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			m := re.FindStringSubmatch(line)
			if m != nil && regexp.MustCompile(`\b(test|lint|build|typecheck|check)\b`).MatchString(m[1]) {
				hints = append(hints, CIHint{File: ".github/workflows/" + e.Name(), Command: strings.TrimSpace(m[1])})
				if len(hints) >= 20 {
					return hints
				}
			}
		}
	}
	return hints
}

func DiscoverCommands(root, lockedFile string) (Discovery, error) {
	if lockedFile != "" {
		if b, err := os.ReadFile(lockedFile); err == nil {
			var lock struct {
				Locked   bool      `json:"locked"`
				Commands []Command `json:"commands"`
			}
			if json.Unmarshal(b, &lock) == nil && lock.Locked {
				areas := map[string]bool{}
				for _, c := range lock.Commands {
					a := c.Area
					if a == "" {
						a = "."
					}
					areas[a] = true
				}
				d := Discovery{Locked: true, Commands: lock.Commands}
				for a := range areas {
					d.Areas = append(d.Areas, a)
				}
				slices.Sort(d.Areas)
				return d, nil
			}
		}
	}
	d := Discovery{Areas: DetectAreas(root, 3), Hints: CIHints(root)}
	for _, area := range d.Areas {
		for _, c := range DiscoverArea(root, area) {
			if c.Tier == "SKIPPED" {
				d.Skipped = append(d.Skipped, c)
			} else {
				d.Commands = append(d.Commands, c)
			}
		}
	}
	return d, nil
}

type CommandResult struct {
	Command
	ExitCode    *int            `json:"exitCode"`
	TimedOut    bool            `json:"timedOut"`
	SpawnError  string          `json:"spawnError"`
	DurationMS  int64           `json:"durationMs"`
	StdoutTail  string          `json:"stdoutTail"`
	StderrTail  string          `json:"stderrTail"`
	Signature   []string        `json:"signature"`
	Status      string          `json:"status"`
	OK          bool            `json:"ok"`
	Why         string          `json:"why,omitempty"`
	FailureKind string          `json:"failureKind,omitempty"`
	Delta       *SignatureDelta `json:"delta,omitempty"`
}

// CommandEvent observes selected checks. Completed ladder events include the
// final baseline comparison, so a RED status can still represent an accepted
// existing failure. Events are transient and do not change persisted results.
type CommandEvent struct {
	Kind     string
	Command  Command
	Result   CommandResult
	Baseline bool
}

type callbackWriter struct {
	buf    *bytes.Buffer
	onLine func(string)
}

func (w callbackWriter) Write(p []byte) (int, error) {
	n, err := w.buf.Write(p)
	if w.onLine != nil {
		for _, line := range strings.Split(string(p), "\n") {
			if strings.TrimSpace(line) != "" {
				w.onLine(line)
			}
		}
	}
	return n, err
}
func tail(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}
func terminateGroup(pid int, signal syscall.Signal) {
	if pid > 0 {
		_ = syscall.Kill(-pid, signal)
	}
}

// RunCommand starts an argv vector in its own process group. Cancellation and
// timeout signal the group, then escalate after ten seconds.
func RunCommand(parent context.Context, c Command, wt string, onLine func(string)) CommandResult {
	r := CommandResult{Command: c}
	if err := parent.Err(); err != nil {
		r.SpawnError = err.Error()
		r.Status = StatusUnrunnable
		return r
	}
	started := time.Now()
	if len(c.Argv) == 0 {
		r.SpawnError = "EMPTY_ARGV"
		r.Status = StatusUnrunnable
		return r
	}
	limit := time.Duration(c.TimeoutMS) * time.Millisecond
	if limit <= 0 {
		limit = tierTimeout[c.Tier]
	}
	if limit <= 0 {
		limit = 15 * time.Minute
	}
	ctx, cancel := context.WithTimeout(parent, limit)
	defer cancel()
	cmd := exec.Command(c.Argv[0], c.Argv[1:]...)
	cmd.Dir = filepath.Join(wt, c.Cwd)
	cmd.Env = append(os.Environ(), "CI=1", "TZ=UTC", "FORCE_COLOR=0", "NO_COLOR=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var out, stderr bytes.Buffer
	cmd.Stdout = callbackWriter{&out, onLine}
	cmd.Stderr = callbackWriter{&stderr, onLine}
	if err := cmd.Start(); err != nil {
		r.SpawnError = err.Error()
		r.StderrTail = tail(err.Error(), 4096)
		r.DurationMS = time.Since(started).Milliseconds()
		r.Status = StatusUnrunnable
		return r
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		r.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
		terminateGroup(cmd.Process.Pid, syscall.SIGTERM)
		killer := time.NewTimer(10 * time.Second)
		select {
		case err = <-done:
			if !killer.Stop() {
				<-killer.C
			}
		case <-killer.C:
			terminateGroup(cmd.Process.Pid, syscall.SIGKILL)
			err = <-done
		}
		// The leader can exit before descendants. Send KILL once more to a still
		// existing group, preventing a child from surviving the timeout.
		terminateGroup(cmd.Process.Pid, syscall.SIGKILL)
	}
	if err == nil {
		zero := 0
		r.ExitCode = &zero
	} else if exit, ok := err.(*exec.ExitError); ok && !r.TimedOut {
		code := exit.ExitCode()
		r.ExitCode = &code
	} else if !r.TimedOut && ctx.Err() == nil {
		r.SpawnError = err.Error()
	}
	r.DurationMS = time.Since(started).Milliseconds()
	r.StdoutTail = tail(out.String(), 4096)
	r.StderrTail = tail(stderr.String(), 4096)
	r.Signature = ExtractSignature(out.String() + "\n" + stderr.String())
	r.Status = Classify(r)
	return r
}

var signatureRules = []struct {
	re     *regexp.Regexp
	prefix string
}{
	{regexp.MustCompile(`([\w./\-]+\.(?:ts|tsx|js|jsx|mjs|cjs|go|py|rs|java|rb|kt|kts|scala|groovy|swift)):(\d+)`), ""},
	{regexp.MustCompile(`([\w./\-]+\.(?:java|kt|kts|scala|groovy)):\[(\d+),\d+\]`), ""},
	{regexp.MustCompile(`([\w./\-]+\.kts?):\s*\((\d+),\s*\d+\)`), ""},
	{regexp.MustCompile(`\b(TS\d{4}|E\d{4}|SA\d{4}|L\d{4}|B\d{4})\b`), ""},
	{regexp.MustCompile(`---\s+FAIL:\s+([A-Za-z0-9_/]+)`), "GOFAIL:"},
	{regexp.MustCompile(`(?m)^not ok \d+ [-–] (.+)$`), "TAP:"},
}
var junitSignatureRE = regexp.MustCompile(`(?m)^\s*([\w.$]+(?:\s*>\s*[^\n>]+?)+)\s+FAILED\s*$`)
var surefireSignatureRE = regexp.MustCompile(`([\w.$]+(?:\([\w.$]+\))?)\s+(?:--\s+)?Time elapsed:[^\n]*<<<\s*(?:FAILURE|ERROR)!`)
var reporterSignatureRE = regexp.MustCompile(`(?m)^\s*[✖✗×]\s+(.+?)(?:\s+\(\d|$)`)

func ExtractSignature(text string) []string {
	set := map[string]bool{}
	for i, rule := range signatureRules {
		for _, m := range rule.re.FindAllStringSubmatch(text, -1) {
			value := rule.prefix + m[1]
			if i < 3 {
				value += ":" + m[2]
			}
			set[value] = true
		}
	}
	for _, m := range junitSignatureRE.FindAllStringSubmatch(text, -1) {
		set["JUNIT:"+strings.Join(strings.Fields(m[1]), " ")] = true
	}
	for _, m := range surefireSignatureRE.FindAllStringSubmatch(text, -1) {
		set["SUREFIRE:"+m[1]] = true
	}
	for _, m := range reporterSignatureRE.FindAllStringSubmatch(text, -1) {
		set["FAIL:"+strings.TrimSpace(m[1])] = true
	}
	result := make([]string, 0, len(set))
	for s := range set {
		result = append(result, s)
	}
	slices.Sort(result)
	return result
}
func Classify(r CommandResult) string {
	if r.SpawnError != "" {
		return StatusUnrunnable
	}
	if r.TimedOut {
		return StatusTimeout
	}
	if r.ExitCode != nil && *r.ExitCode == 0 {
		return StatusGreen
	}
	if len(r.Signature) == 0 {
		return StatusOpaque
	}
	return StatusRed
}

type SignatureDelta struct {
	OK      bool     `json:"ok"`
	Added   []string `json:"added"`
	Drifted []string `json:"drifted"`
	Removed []string `json:"removed"`
}

var signatureFileRE = regexp.MustCompile(`^(.+):(\d+)$`)

func CompareSignatures(before, after []string) SignatureDelta {
	d := SignatureDelta{OK: true}
	b := map[string]bool{}
	files := map[string]bool{}
	for _, s := range before {
		b[s] = true
		if m := signatureFileRE.FindStringSubmatch(s); m != nil {
			files[m[1]] = true
		}
	}
	for _, s := range after {
		if b[s] {
			continue
		}
		if m := signatureFileRE.FindStringSubmatch(s); m != nil && files[m[1]] {
			d.Drifted = append(d.Drifted, s)
		} else {
			d.Added = append(d.Added, s)
		}
	}
	for _, s := range before {
		if !slices.Contains(after, s) {
			d.Removed = append(d.Removed, s)
		}
	}
	d.OK = len(d.Added) == 0
	return d
}

type Baseline struct {
	Results    []CommandResult `json:"results"`
	Green      int             `json:"green"`
	Red        int             `json:"red"`
	Unrunnable []CommandResult `json:"unrunnable"`
	Opaque     []CommandResult `json:"opaque"`
	Usable     bool            `json:"usable"`
	Describe   string          `json:"describe"`
}

func RunBaseline(ctx context.Context, commands []Command, wt string, observer func(CommandEvent)) Baseline {
	b := Baseline{}
	for _, c := range commands {
		if observer != nil {
			observer(CommandEvent{Kind: "started", Command: c, Baseline: true})
		}
		r := RunCommand(ctx, c, wt, nil)
		b.Results = append(b.Results, r)
		switch r.Status {
		case StatusGreen:
			b.Green++
		case StatusRed:
			b.Red++
		case StatusUnrunnable:
			b.Unrunnable = append(b.Unrunnable, r)
		case StatusOpaque:
			b.Opaque = append(b.Opaque, r)
		}
		if observer != nil {
			observer(CommandEvent{Kind: "completed", Command: c, Result: r, Baseline: true})
		}
	}
	b.Usable = b.Green > 0
	b.Describe = fmt.Sprintf("GREEN %d / RED-differential %d of %d", b.Green, b.Red, len(b.Results))
	return b
}
func AreasForPaths(areas, paths []string) []string {
	hit := map[string]bool{}
	for _, p := range paths {
		best := "."
		for _, a := range areas {
			if a != "." && (p == a || strings.HasPrefix(p, a+"/")) && len(a) > len(best) {
				best = a
			}
		}
		hit[best] = true
	}
	if slices.Contains(areas, ".") {
		hit["."] = true
	}
	out := make([]string, 0, len(hit))
	for a := range hit {
		out = append(out, a)
	}
	slices.Sort(out)
	return out
}

type Ladder struct {
	OK     bool            `json:"ok"`
	Checks []CommandResult `json:"checks"`
	Failed *CommandResult  `json:"failed"`
	Scope  []string        `json:"scope"`
}

func RunLadder(ctx context.Context, baseline Baseline, commands []Command, wt string, changed, areas []string, observer func(CommandEvent)) Ladder {
	scope := AreasForPaths(areas, changed)
	result := Ladder{OK: true, Scope: scope}
	byID := map[string]CommandResult{}
	for _, r := range baseline.Results {
		byID[r.ID] = r
	}
	for _, c := range commands {
		if !slices.Contains(scope, c.Area) {
			continue
		}
		base, ok := byID[c.ID]
		if ok && (base.Status == StatusTimeout || base.Status == StatusUnrunnable || base.Status == StatusOpaque) {
			continue
		}
		if observer != nil {
			observer(CommandEvent{Kind: "started", Command: c})
		}
		r := RunCommand(ctx, c, wt, nil)
		switch {
		case r.SpawnError != "":
			r.Why = "command could not be executed"
			r.FailureKind = "VALIDATION_UNRUNNABLE"
		case r.TimedOut:
			r.Why = "command timed out"
			r.FailureKind = "VALIDATION_TIMEOUT"
		case !ok || base.Status == StatusGreen:
			r.OK = r.ExitCode != nil && *r.ExitCode == 0
			if !r.OK {
				r.Why = "was GREEN at baseline"
				r.FailureKind = "REGRESSION"
			}
		case r.Status == StatusOpaque:
			r.Why = "failed with no recognisable output"
			r.FailureKind = "REGRESSION"
		default:
			d := CompareSignatures(base.Signature, r.Signature)
			r.Delta = &d
			r.OK = d.OK
			if !r.OK {
				r.Why = "new failure signature"
				r.FailureKind = "REGRESSION"
			}
		}
		result.Checks = append(result.Checks, r)
		if observer != nil {
			observer(CommandEvent{Kind: "completed", Command: c, Result: r})
		}
		if !r.OK {
			result.OK = false
			result.Failed = &result.Checks[len(result.Checks)-1]
			break
		}
	}
	return result
}

// RunArgv is a small process-group runner available to the provider adapter.
func RunArgv(ctx context.Context, argv []string, cwd string, env []string, stdout, stderr io.Writer, limit time.Duration) error {
	if len(argv) == 0 {
		return fmt.Errorf("empty argv")
	}
	if limit <= 0 {
		limit = 15 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = cwd
	cmd.Env = env
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-runCtx.Done():
		terminateGroup(cmd.Process.Pid, syscall.SIGTERM)
		timer := time.NewTimer(10 * time.Second)
		select {
		case <-done:
			timer.Stop()
		case <-timer.C:
			terminateGroup(cmd.Process.Pid, syscall.SIGKILL)
			<-done
		}
		terminateGroup(cmd.Process.Pid, syscall.SIGKILL)
		return runCtx.Err()
	}
}
