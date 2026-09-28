package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var sourceRE = regexp.MustCompile(`\.(ts|tsx|js|jsx|mjs|cjs|go|py|rs|java|kt|rb|swift|vue|svelte)$`)
var vendoredRE = regexp.MustCompile(`(^|/)(node_modules|vendor|dist|build)/`)
var publishRE = regexp.MustCompile(`\b(npm|pnpm|yarn|bun)\s+publish\b|softprops/action-gh-release|goreleaser|cargo\s+publish|twine\s+upload`)
var moduleRE = regexp.MustCompile(`(?m)^module\s+(\S+)`)
var publicModuleRE = regexp.MustCompile(`^(github|gitlab|bitbucket|golang\.org|gopkg\.in)\.`)

var ForbiddenGlobs = []string{
	".git", ".git/**", "**/package.json", "**/package-lock.json", "**/pnpm-lock.yaml", "**/yarn.lock",
	"**/bun.lock", "**/bun.lockb", "**/npm-shrinkwrap.json", "**/go.mod", "**/go.sum", "**/Cargo.toml", "**/Cargo.lock",
	"**/pyproject.toml", "**/poetry.lock", "**/uv.lock", "**/Pipfile.lock", "**/requirements*.txt", "**/Gemfile", "**/Gemfile.lock", "**/composer.json", "**/composer.lock", "**/*.lock",
	"**/*.gradle", "**/*.gradle.kts", "**/gradle.properties", "**/gradle/wrapper/**", "**/gradlew", "**/gradlew.bat", "**/gradle/libs.versions.toml", "**/gradle.lockfile", "**/gradle/dependency-locks/**",
	"**/pom.xml", "**/.mvn/**", "**/mvnw", "**/mvnw.cmd", "**/__snapshots__/**", "**/*.snap", "**/*.ambr", "**/__image_snapshots__/**", "**/testdata/**", "**/golden/**", "**/*.golden", "**/fixtures/**", "**/src/test/resources/**",
	"*.png", "*.jpg", "*.jpeg", "*.gif", "*.webp", "*.svg", "*.ico", "*.pdf", "*.woff", "*.woff2", "*.wasm", "*.zip", "*.tar", "*.gz",
	".github/**", ".gitlab-ci.yml", "Jenkinsfile", "**/Dockerfile*", "**/compose*.yml", "**/compose*.yaml", "**/*.tf", "**/*.tfstate", "**/migrations/**", "**/db/migrations/**",
	".env", ".env.*", "**/.env", "**/.env.*", ".refactor/**", ".agents/**", ".claude/**", "AGENTS.md", "CLAUDE.md", "**/AGENTS.md", "**/CLAUDE.md",
	"**/openapi.yaml", "**/openapi.yml", "**/openapi.json", "**/*.proto", "**/schema.graphql", "**/generated/**", "**/*.gen.go", "**/*_gen.go", "**/*.pb.go", "LICENSE",
}

type Surface struct {
	Verdict string
	Reasons []string
	Notes   []string
}

func ExternalSurface(wt string) Surface {
	s := Surface{Reasons: []string{}, Notes: []string{}}
	if raw, err := os.ReadFile(filepath.Join(wt, "package.json")); err == nil {
		var pkg map[string]any
		if json.Unmarshal(raw, &pkg) == nil {
			if pkg["private"] == true {
				s.Notes = append(s.Notes, "package.json declares private: true")
			} else {
				s.Reasons = append(s.Reasons, "package.json does not declare private: true, so it could be published")
			}
			for _, field := range []string{"exports", "main", "module", "bin", "types", "files"} {
				if _, ok := pkg[field]; ok {
					s.Reasons = append(s.Reasons, fmt.Sprintf("package.json declares %q, a public entrypoint", field))
				}
			}
			if pkg["publishConfig"] != nil {
				s.Reasons = append(s.Reasons, "package.json declares publishConfig")
			}
			if pkg["workspaces"] != nil {
				s.Notes = append(s.Notes, "workspaces present: sibling packages are in-repository consumers, and are covered by the file inventory")
			}
		}
	}
	for _, f := range []string{".npmrc", ".yarnrc.yml"} {
		if _, err := os.Stat(filepath.Join(wt, f)); err == nil {
			s.Notes = append(s.Notes, f+" present (registry configuration, not proof of publishing)")
		}
	}
	if entries, err := os.ReadDir(filepath.Join(wt, ".github", "workflows")); err == nil {
		for _, entry := range entries {
			if raw, err := os.ReadFile(filepath.Join(wt, ".github", "workflows", entry.Name())); err == nil && publishRE.Match(raw) {
				s.Reasons = append(s.Reasons, ".github/workflows/"+entry.Name()+" publishes an artifact")
			}
		}
	}
	if raw, err := os.ReadFile(filepath.Join(wt, "go.mod")); err == nil {
		if m := moduleRE.FindSubmatch(raw); m != nil && publicModuleRE.Match(m[1]) {
			s.Reasons = append(s.Reasons, "go.mod module path "+string(m[1])+" is importable by other Go modules")
		}
	}
	s.Verdict = "NONE_DETECTED"
	if len(s.Reasons) > 0 {
		s.Verdict = "POSSIBLE"
	}
	return s
}

type CommandFact struct {
	Area   string
	Tier   string
	Name   string
	Argv   []string
	CWD    string
	Source string
	Reason string
}
type FactsOptions struct {
	Areas    []string
	Commands []CommandFact
	Skipped  []CommandFact
	MaxFiles int
	Targets  []string
}
type fileFact struct {
	path  string
	lines int
}

func underTarget(path string, targets []string) bool {
	if len(targets) == 0 {
		return true
	}
	for _, t := range targets {
		if path == t || strings.HasPrefix(path, t+"/") {
			return true
		}
	}
	return false
}
func lineCount(wt, rel string) int {
	raw, err := os.ReadFile(filepath.Join(wt, rel))
	if err != nil || bytes.IndexByte(raw, 0) >= 0 {
		return 0
	}
	if len(raw) == 0 {
		return 0
	}
	n := bytes.Count(raw, []byte{'\n'})
	if raw[len(raw)-1] != '\n' {
		n++
	}
	return n
}

// CollectRepoFacts provides the byte-identical repository inventory both
// providers receive. Git is invoked without a shell; no repository file is written.
func CollectRepoFacts(wt string, opts FactsOptions) (string, error) {
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = wt
	raw, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("list tracked files for audit: %w", err)
	}
	files := make([]fileFact, 0)
	for _, f := range bytes.Split(raw, []byte{0}) {
		if len(f) == 0 {
			continue
		}
		p := string(f)
		if sourceRE.MatchString(p) && !vendoredRE.MatchString(p) {
			files = append(files, fileFact{path: p, lines: lineCount(wt, p)})
		}
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].lines > files[j].lines })
	scoped := make([]fileFact, 0, len(files))
	for _, f := range files {
		if underTarget(f.path, opts.Targets) {
			scoped = append(scoped, f)
		}
	}
	maxFiles := opts.MaxFiles
	if maxFiles <= 0 {
		maxFiles = 400
	}
	big := []string{}
	for _, f := range scoped {
		if f.lines >= 500 {
			big = append(big, fmt.Sprintf("%s:%d", f.path, f.lines))
		}
	}
	listing := []string{}
	for _, f := range scoped[:min(maxFiles, len(scoped))] {
		listing = append(listing, fmt.Sprintf("%5d  %s", f.lines, f.path))
	}
	ext := ExternalSurface(wt)
	extLines := []string{"external distribution: " + ext.Verdict}
	if ext.Verdict == "NONE_DETECTED" {
		extLines = append(extLines,
			"  The orchestrator checked the manifest, publish configuration and release workflows and found",
			"  nothing that distributes this code outside the repository. For this run, treat",
			"  external_consumer_risk as NONE unless YOU find a concrete distribution path this check missed.",
			"  Do not answer UNKNOWN merely because you cannot personally inspect a registry.")
	} else {
		extLines = append(extLines, "  This repository may be consumed from outside. Treat exported symbols accordingly:")
		for _, r := range ext.Reasons {
			extLines = append(extLines, "    - "+r)
		}
	}
	for _, n := range ext.Notes {
		extLines = append(extLines, "  note: "+n)
	}
	var cmdLines []string
	for _, c := range opts.Commands {
		cmdLines = append(cmdLines, fmt.Sprintf("  %s %s %s: %s  (cwd %s, from %s)", c.Area, c.Tier, c.Name, strings.Join(c.Argv, " "), c.CWD, c.Source))
	}
	if len(cmdLines) == 0 {
		cmdLines = []string{"  (none discovered)"}
	}
	var skipped []string
	for _, c := range opts.Skipped {
		skipped = append(skipped, fmt.Sprintf("  %s %s: %s — NOT RUN (%s)", c.Area, c.Name, strings.Join(c.Argv, " "), c.Reason))
	}
	out := []string{"repository areas: " + strings.Join(opts.Areas, ", ")}
	if len(opts.Targets) > 0 {
		out = append(out, "scan scope: "+strings.Join(opts.Targets, ", "), "  Survey ONLY these paths when looking for candidates. The file inventory below", "  is restricted to them for that reason.", fmt.Sprintf("  The other %d tracked source file(s) are NOT listed but ARE reachable with", len(files)-len(scoped)), "  Glob and Grep, and MAY be modified when a candidate rooted in the scope requires", "  it — a change that stopped at a directory boundary would not compile. Use them", "  freely as evidence; do not go looking for problems there.", "")
	}
	title := "tracked source files"
	if len(opts.Targets) > 0 {
		title += " in scope"
	}
	out = append(out, fmt.Sprintf("%s: %d (largest %d listed below, lines first)", title, len(scoped), maxFiles))
	if len(big) > 0 {
		upto := big[:min(10, len(big))]
		out = append(out, fmt.Sprintf("files at or over 500 lines: %d (%s)", len(big), strings.Join(upto, ", ")))
	} else {
		out = append(out, "files at or over 500 lines: none")
	}
	out = append(out, "", strings.Join(extLines, "\n"), "", "paths the orchestrator will REJECT if a change touches them — never build a", "candidate whose work requires editing one of these:")
	for _, g := range ForbiddenGlobs {
		out = append(out, "  "+g)
	}
	out = append(out, "", "validation commands the orchestrator will run for you (you do not run them):", strings.Join(cmdLines, "\n"))
	if len(skipped) > 0 {
		out = append(out, "\ncommands deliberately excluded from the loop:\n"+strings.Join(skipped, "\n"))
	}
	out = append(out, "", "file inventory (lines, path):", strings.Join(listing, "\n"))
	return strings.Join(out, "\n"), nil
}

var descriptionRE = regexp.MustCompile(`(?m)^description:\s*["']?(.*?)["']?\s*$`)
var whitespaceRE = regexp.MustCompile(`\s+`)

// CollectAreaSkills surfaces nested skill instructions for files in scope.
func CollectAreaSkills(wt string, scopePaths []string) string {
	var roots []string
	var walk func(string, int)
	walk = func(dir string, depth int) {
		if depth > 3 {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if name == "node_modules" || name == ".git" || name == "dist" {
				continue
			}
			abs := filepath.Join(dir, name)
			if name == ".agents" {
				if st, err := os.Stat(filepath.Join(abs, "skills")); err == nil && st.IsDir() {
					roots = append(roots, filepath.Join(abs, "skills"))
				}
			} else if !strings.HasPrefix(name, ".") {
				walk(abs, depth+1)
			}
		}
	}
	walk(wt, 0)
	var out []string
	for _, root := range roots {
		areaRel, err := filepath.Rel(wt, filepath.Dir(filepath.Dir(root)))
		if err != nil || areaRel == "." {
			continue
		}
		relevant := false
		for _, p := range scopePaths {
			if p == areaRel || strings.HasPrefix(p, areaRel+string(filepath.Separator)) || strings.HasPrefix(p, filepath.ToSlash(areaRel)+"/") {
				relevant = true
				break
			}
		}
		if !relevant {
			continue
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, e := range entries {
			if !e.IsDir() && e.Type()&os.ModeSymlink == 0 {
				continue
			}
			file := filepath.Join(root, e.Name(), "SKILL.md")
			raw, err := os.ReadFile(file)
			if err != nil {
				continue
			}
			if len(raw) > 1200 {
				raw = raw[:1200]
			}
			desc := ""
			if m := descriptionRE.FindSubmatch(raw); m != nil {
				desc = whitespaceRE.ReplaceAllString(string(m[1]), " ")
				if len(desc) > 140 {
					desc = desc[:140]
				}
			}
			out = append(out, fmt.Sprintf("  %s  %s\n    %s\n    %s", filepath.ToSlash(areaRel), e.Name(), file, desc))
		}
	}
	if len(out) == 0 {
		return "(no area-specific skills apply to the files in scope)"
	}
	return strings.Join(out, "\n")
}
