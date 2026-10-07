package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var ForbiddenGlobs = []string{
	".git", ".git/**", "**/package.json", "**/package-lock.json", "**/pnpm-lock.yaml", "**/yarn.lock", "**/bun.lock", "**/bun.lockb", "**/npm-shrinkwrap.json",
	"**/go.mod", "**/go.sum", "**/Cargo.toml", "**/Cargo.lock", "**/pyproject.toml", "**/poetry.lock", "**/uv.lock", "**/Pipfile.lock", "**/requirements*.txt",
	"**/Gemfile", "**/Gemfile.lock", "**/composer.json", "**/composer.lock", "**/*.lock", "**/*.gradle", "**/*.gradle.kts", "**/gradle.properties", "**/gradle/wrapper/**", "**/gradlew", "**/gradlew.bat", "**/gradle/libs.versions.toml", "**/gradle.lockfile", "**/gradle/dependency-locks/**", "**/pom.xml", "**/.mvn/**", "**/mvnw", "**/mvnw.cmd",
	"**/__snapshots__/**", "**/*.snap", "**/*.ambr", "**/__image_snapshots__/**", "**/testdata/**", "**/golden/**", "**/*.golden", "**/fixtures/**", "**/src/test/resources/**",
	"*.png", "*.jpg", "*.jpeg", "*.gif", "*.webp", "*.svg", "*.ico", "*.pdf", "*.woff", "*.woff2", "*.wasm", "*.zip", "*.tar", "*.gz",
	".github/**", ".gitlab-ci.yml", "Jenkinsfile", "**/Dockerfile*", "**/compose*.yml", "**/compose*.yaml", "**/*.tf", "**/*.tfstate", "**/migrations/**", "**/db/migrations/**",
	".env", ".env.*", "**/.env", "**/.env.*", ".refactor/**", ".agents/**", ".claude/**", "AGENTS.md", "CLAUDE.md", "**/AGENTS.md", "**/CLAUDE.md", "**/openapi.yaml", "**/openapi.yml", "**/openapi.json", "**/*.proto", "**/schema.graphql", "**/generated/**", "**/*.gen.go", "**/*_gen.go", "**/*.pb.go", "LICENSE",
}
var TestGlobs = []string{"**/*.test.*", "**/*.spec.*", "**/test/**", "**/tests/**", "**/__tests__/**", "**/*_test.go", "**/test_*.py", "**/*_test.py", "**/*_test.rs", "**/src/*Test/**", "**/src/*test/**", "**/*Test.java", "**/*Tests.java", "**/*TestCase.java", "**/*IT.java", "**/*Test.kt", "**/*Tests.kt", "**/*Spec.kt", "**/*Spec.groovy"}
var weakeningRE = regexp.MustCompile(`\.skip\(|\.only\(|\bxit\(|\bxdescribe\(|\bt\.Skip\(|@pytest\.mark\.skip|#\[ignore\]|\bt\.SkipNow\(|@Disabled\b|@Ignore\b|\bassume(?:True|False|That|NotNull)\s*\(|\benabled\s*=\s*false\b`)

func GlobRegexp(glob string) (*regexp.Regexp, error) {
	if !strings.Contains(glob, "/") {
		glob = "**/" + glob
	}
	var b strings.Builder
	b.WriteByte('^')
	for i := 0; i < len(glob); {
		switch glob[i] {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				i += 2
				if i < len(glob) && glob[i] == '/' {
					b.WriteString("(?:[^/]+/)*")
					i++
				} else {
					b.WriteString(".*")
				}
			} else {
				b.WriteString("[^/]*")
				i++
			}
		case '?':
			b.WriteString("[^/]")
			i++
		default:
			b.WriteString(regexp.QuoteMeta(glob[i : i+1]))
			i++
		}
	}
	b.WriteByte('$')
	return regexp.Compile(b.String())
}
func matchesGlobs(path string, globs []string) bool {
	for _, g := range globs {
		re, err := GlobRegexp(g)
		if err == nil && re.MatchString(path) {
			return true
		}
	}
	return false
}
func IsTestPath(path string, extra []string) bool {
	return matchesGlobs(path, append(slices.Clone(TestGlobs), extra...))
}

type Packet struct {
	Category      string   `json:"category"`
	Allowlist     []string `json:"allowlist"`
	OriginalLines int      `json:"original_lines"`
}
type Policy struct {
	MaxFilesPerCandidate int      `json:"max_files_per_candidate"`
	MaxChangedLines      int      `json:"max_changed_lines"`
	ExtraForbiddenGlobs  []string `json:"extra_forbidden_globs"`
	ExtraTestGlobs       []string `json:"extra_test_globs"`
}
type FileStat struct {
	Insertions, Deletions int
	Binary                bool
}
type GateFacts struct {
	WorktreeHead, SourceHead, SourceFingerprintNow string
	PreOID, BaseOID, SourceFingerprintBefore       string
	Changed, Deleted                               []string
	Insertions, Deletions                          int
	Binary                                         []string
	PerFile                                        map[string]FileStat
	AddedByFile                                    map[string][]string
	PrimaryFileLines                               *int
	ProspectiveTree                                string
}
type GateCheck struct {
	ID     string   `json:"id"`
	OK     bool     `json:"ok"`
	Code   string   `json:"code,omitempty"`
	Detail string   `json:"detail"`
	Halt   bool     `json:"halt,omitempty"`
	Paths  []string `json:"paths,omitempty"`
}
type GateResult struct {
	Verdict   string      `json:"verdict"`
	Checks    []GateCheck `json:"checks"`
	Violation *GateCheck  `json:"violation"`
}

func countLines(path string) *int {
	b, err := os.ReadFile(path)
	if err != nil || slices.Contains(b, byte(0)) {
		return nil
	}
	n := strings.Count(string(b), "\n")
	if len(b) > 0 && b[len(b)-1] != '\n' {
		n++
	}
	return &n
}

func CollectFacts(wt, root, preOID, baseOID, sourceFingerprint string, packet Packet) (GateFacts, error) {
	f := GateFacts{PreOID: preOID, BaseOID: baseOID, SourceFingerprintBefore: sourceFingerprint, PerFile: map[string]FileStat{}, AddedByFile: map[string][]string{}}
	var err error
	if f.Changed, err = ChangedPaths(wt); err != nil {
		return f, err
	}
	if err = StageExact(wt, f.Changed); err != nil {
		return f, err
	}
	if f.Deleted, err = DeletedPaths(wt); err != nil {
		return f, err
	}
	if f.WorktreeHead, err = HeadOID(wt); err != nil {
		return f, err
	}
	if f.SourceHead, err = HeadOID(root); err != nil {
		return f, err
	}
	if f.SourceFingerprintNow, err = SourceFingerprint(root); err != nil {
		return f, err
	}
	numstat, err := Git(wt, "diff", "--numstat", "-z", "--no-renames", "HEAD")
	if err != nil {
		return f, err
	}
	for _, line := range nulPaths(numstat) {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		p := parts[2]
		if parts[0] == "-" || parts[1] == "-" {
			f.Binary = append(f.Binary, p)
			f.PerFile[p] = FileStat{Binary: true}
			continue
		}
		ins, _ := strconv.Atoi(parts[0])
		del, _ := strconv.Atoi(parts[1])
		f.Insertions += ins
		f.Deletions += del
		f.PerFile[p] = FileStat{Insertions: ins, Deletions: del}
	}
	for _, p := range f.Changed {
		if IsTestPath(p, nil) {
			out, err := Git(wt, "diff", "--unified=0", "HEAD", "--", p)
			if err != nil {
				return f, err
			}
			for _, line := range strings.Split(out, "\n") {
				if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
					f.AddedByFile[p] = append(f.AddedByFile[p], strings.TrimPrefix(line, "+"))
				}
			}
		}
	}
	if len(packet.Allowlist) > 0 {
		f.PrimaryFileLines = countLines(filepath.Join(wt, packet.Allowlist[0]))
	}
	if f.ProspectiveTree, err = WriteTree(wt); err != nil {
		return f, err
	}
	return f, nil
}

func ChangedLineBudget(packet Packet, policy Policy) int {
	n := policy.MaxChangedLines
	if n <= 0 {
		n = 600
	}
	if packet.Category == "LARGE_COMPONENT_SPLIT" {
		derived := (packet.OriginalLines*5 + 1) / 2
		if derived > n {
			n = derived
		}
	}
	return n
}
func NetSizeRule(packet Packet, facts GateFacts) (bool, string) {
	net := facts.Insertions - facts.Deletions
	switch packet.Category {
	case "DEAD_CODE", "COMPATIBILITY_REMOVAL":
		return net <= 5, fmt.Sprintf("net %d (limit +5)", net)
	case "DEDUPLICATION":
		return net <= 0, fmt.Sprintf("net %d (limit 0)", net)
	case "LARGE_COMPONENT_SPLIT":
		budget := (packet.OriginalLines + 5) / 10
		if budget < 20 {
			budget = 20
		}
		if net > budget || net < -budget {
			return false, fmt.Sprintf("net %d exceeds ±%d", net, budget)
		}
		if facts.PrimaryFileLines != nil && *facts.PrimaryFileLines >= 500 {
			return false, fmt.Sprintf("primary file still %d lines", *facts.PrimaryFileLines)
		}
		return true, fmt.Sprintf("net %d within ±%d", net, budget)
	default:
		return true, "no net-size rule"
	}
}

// RunCharacterizationChecks applies the measured-diff safety gates without a
// production refactor's net-size rule or target requirement: tests may live
// outside the production target, but every changed path must be a test.
func RunCharacterizationChecks(f GateFacts, files []string, policy Policy, treeHashes []string) GateResult {
	r := RunChecks(f, Packet{Allowlist: files}, policy, treeHashes, nil)
	if r.Verdict != "PASS" {
		return r
	}
	check := GateCheck{ID: "TEST_ONLY", OK: true, Detail: "only test files changed"}
	for _, path := range f.Changed {
		if !IsTestPath(path, policy.ExtraTestGlobs) {
			check.Paths = append(check.Paths, path)
		}
	}
	if len(check.Paths) > 0 {
		check.OK, check.Code = false, "PRODUCTION_CHANGED"
		check.Detail = "characterization changed non-test paths: " + strings.Join(check.Paths, ", ")
		r.Verdict, r.Violation = "VIOLATION", &check
	}
	r.Checks = append(r.Checks, check)
	return r
}

func RunChecks(f GateFacts, p Packet, policy Policy, treeHashes, targets []string) GateResult {
	r := GateResult{Verdict: "PASS", Checks: []GateCheck{}}
	add := func(c GateCheck) bool {
		r.Checks = append(r.Checks, c)
		if !c.OK {
			r.Violation = &c
			if c.Halt {
				r.Verdict = "HALT"
			} else {
				r.Verdict = "VIOLATION"
			}
			return false
		}
		return true
	}
	good := func(id, detail string) GateCheck { return GateCheck{ID: id, OK: true, Detail: detail} }
	bad := func(id, code, detail string, halt bool, paths []string) GateCheck {
		return GateCheck{ID: id, Code: code, Detail: detail, Halt: halt, Paths: paths}
	}
	if !add(GateCheck{ID: "WORKTREE_HEAD_STABLE", OK: f.WorktreeHead == f.PreOID, Code: func() string {
		if f.WorktreeHead != f.PreOID {
			return "WORKTREE_HEAD_MOVED"
		}
		return ""
	}(), Halt: f.WorktreeHead != f.PreOID, Detail: "worktree HEAD unchanged"}) {
		return r
	}
	if f.SourceHead != f.BaseOID || f.SourceFingerprintNow != f.SourceFingerprintBefore {
		add(bad("SOURCE_UNTOUCHED", "SOURCE_MUTATED", "source checkout changed during write phase", true, nil))
		return r
	}
	add(good("SOURCE_UNTOUCHED", "source checkout unchanged"))
	add(good("CHANGED_SET", fmt.Sprintf("%d changed, %d deleted", len(f.Changed), len(f.Deleted))))
	if len(f.Changed) == 0 {
		add(good("NON_EMPTY", "no changes produced"))
		r.Verdict = "NO_OP"
		return r
	}
	add(good("NON_EMPTY", fmt.Sprintf("%d changed", len(f.Changed))))
	forbidden := append(slices.Clone(ForbiddenGlobs), policy.ExtraForbiddenGlobs...)
	var hits []string
	for _, path := range f.Changed {
		if matchesGlobs(path, forbidden) {
			hits = append(hits, path)
		}
	}
	if len(hits) > 0 {
		add(bad("FORBIDDEN_PATH", "FORBIDDEN_PATH", strings.Join(hits, ", "), false, hits))
		return r
	}
	if len(f.Binary) > 0 {
		add(bad("FORBIDDEN_PATH", "FORBIDDEN_BINARY", strings.Join(f.Binary, ", "), false, f.Binary))
		return r
	}
	add(good("FORBIDDEN_PATH", "no forbidden path"))
	allow := map[string]bool{}
	dirs := map[string]bool{}
	exts := map[string]bool{}
	for _, path := range p.Allowlist {
		allow[path] = true
		dirs[filepath.ToSlash(filepath.Dir(path))] = true
		exts[filepath.Ext(path)] = true
	}
	var outside []string
	for _, path := range f.Changed {
		if allow[path] {
			continue
		}
		if p.Category == "LARGE_COMPONENT_SPLIT" && dirs[filepath.ToSlash(filepath.Dir(path))] && exts[filepath.Ext(path)] {
			continue
		}
		outside = append(outside, path)
	}
	if len(outside) > 0 {
		add(bad("ALLOWLIST", "OUT_OF_SCOPE", strings.Join(outside, ", "), false, outside))
		return r
	}
	add(good("ALLOWLIST", "changed paths allowed"))
	if len(targets) > 0 && !TouchesTarget(f.Changed, targets) {
		add(bad("TARGET_SCOPE", "TARGET_SCOPE_EMPTY", "no change under target", false, nil))
		return r
	}
	add(good("TARGET_SCOPE", "target touched"))
	maxFiles := policy.MaxFilesPerCandidate
	if maxFiles <= 0 {
		maxFiles = 8
	}
	if len(f.Changed) > maxFiles || f.Insertions+f.Deletions > ChangedLineBudget(p, policy) {
		add(bad("SIZE_CAPS", "TOO_LARGE", "change exceeds size cap", false, nil))
		return r
	}
	add(good("SIZE_CAPS", "within size caps"))
	if ok, detail := NetSizeRule(p, f); !ok {
		add(bad("NET_SIZE", "NET_SIZE_RULE", detail, false, nil))
		return r
	} else {
		add(good("NET_SIZE", detail))
	}
	var problems []string
	for _, path := range f.Deleted {
		if IsTestPath(path, policy.ExtraTestGlobs) {
			problems = append(problems, "test deleted: "+path)
		}
	}
	for _, path := range f.Changed {
		if !IsTestPath(path, policy.ExtraTestGlobs) {
			continue
		}
		if f.PerFile[path].Deletions > 0 {
			problems = append(problems, "test lines removed: "+path)
		}
		for _, line := range f.AddedByFile[path] {
			if weakeningRE.MatchString(line) {
				problems = append(problems, "test weakened: "+path)
				break
			}
		}
	}
	if len(problems) > 0 {
		add(bad("TEST_INTEGRITY", "TEST_WEAKENED", strings.Join(problems, "; "), false, nil))
		return r
	}
	add(good("TEST_INTEGRITY", "tests not weakened"))
	if f.ProspectiveTree != "" && slices.Contains(treeHashes, f.ProspectiveTree) {
		add(bad("THRASH", "THRASH_REVERT", "tree already seen", false, nil))
		return r
	}
	add(good("THRASH", "tree not seen"))
	return r
}
