package engine

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
)

//go:embed templates/*.txt
var promptFiles embed.FS

var RequiredSkills = []string{"sharpen-clarify", "sharpen-review", "sharpen-challenge", "sharpen-assess", "sharpen-refine", "sharpen-cold-review", "sharpen-brief", "sharpen-dedupe"}

func NewNonce() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("secure nonce unavailable: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

type SkippedCandidate struct {
	Fingerprint string
	Reason      string
}

// PhaseEvidence contains measured controller facts, never model assertions or
// raw command output. Baseline failures remain failures even when usable.
type PhaseEvidence struct {
	BaseCommit       string                    `json:"base_commit"`
	BaselineSummary  string                    `json:"baseline_summary"`
	BaselineUsable   bool                      `json:"baseline_usable"`
	BaselineChecks   []CheckEvidence           `json:"baseline_checks"`
	Characterization *CharacterizationEvidence `json:"characterization"`
}

type CheckEvidence struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	ExitCode *int   `json:"exit_code"`
}

type CharacterizationEvidence struct {
	Commit             string          `json:"commit"`
	Files              []string        `json:"files"`
	CreatedFiles       []string        `json:"created_files"`
	ValidationAccepted bool            `json:"validation_accepted"`
	Checks             []CheckEvidence `json:"checks"`
}

// PromptArgs carries the phase-specific facts already established by the
// orchestrator. Repository-sourced strings are fenced as untrusted data.
type PromptArgs struct {
	Provider        string
	Nonce           string
	Incremental     bool
	Committed       int
	Cycle           int
	Targets         []string
	RepoFacts       string
	SeenDone        []string
	SeenSkipped     []SkippedCandidate
	Violated        []string
	Candidate       map[string]any
	Packet          map[string]any
	AreaSkills      string
	Commands        string
	BaselineSummary string
	PhaseEvidence   *PhaseEvidence
	Diff            string
	Rationale       string
	DeadProvider    string
	LiveProvider    string
	FailureClass    string
	StateJSON       string
	JournalTail     string
	GitLog          string
}

// BuildPrompt reproduces the model-facing instructions from prompts.mjs.
// The embedded templates were generated from that module and have no runtime
// Node dependency. Caller-supplied repository data is fenced, never obeyed.
func BuildPrompt(phase string, a PromptArgs) (string, error) {
	if a.Provider != "claude" && a.Provider != "codex" {
		return "", fmt.Errorf("unknown prompt provider %q", a.Provider)
	}
	if a.Nonce == "" {
		a.Nonce = NewNonce()
	}
	name := phase
	switch phase {
	case "audit":
		name = "audit_all_initial"
		if len(a.Targets) > 0 {
			name = "audit_scoped_initial"
		}
		if a.Incremental {
			suffix := "uncommitted"
			if a.Committed > 0 {
				suffix = "continued"
			}
			scope := "all"
			if len(a.Targets) > 0 {
				scope = "scoped"
			}
			name = "audit_" + scope + "_" + suffix
		}
	case "deep_check", "deepcheck":
		scope := "all"
		if len(a.Targets) > 0 {
			scope = "scoped"
		}
		kind := "normal"
		if a.Candidate["category"] == "DEDUPLICATION" {
			kind = "dedupe"
		}
		name = "deepcheck_" + scope + "_" + kind
	case "execute":
		name = "execute_normal"
		if a.Packet["category"] == "DEDUPLICATION" {
			name = "execute_dedupe"
		}
	case "characterization", "preflight", "review", "handoff":
	default:
		return "", fmt.Errorf("unsupported prompt phase %q", phase)
	}
	b, err := fs.ReadFile(promptFiles, "templates/"+name+".txt")
	if err != nil {
		return "", fmt.Errorf("read embedded prompt %s: %w", name, err)
	}
	s := string(b)
	jsonOf := func(v any) string { b, _ := json.MarshalIndent(v, "", "  "); return string(b) }
	// Node's JSON.stringify(JSON object, null, 2) retains insertion order.
	// Go's map encoder sorts keys. Ordering is irrelevant to the schema or fence.
	packetJSON := jsonOf(a.Packet)
	candidateJSON := jsonOf(a.Candidate)
	review := map[string]any{}
	for _, key := range []string{"candidate_id", "category", "minimal_change", "allowlist", "contracts"} {
		review[key] = a.Packet[key]
	}
	replacements := []string{
		"__NONCE__", a.Nonce, "__REPO_FACTS__", a.RepoFacts, "__TARGETS__", strings.Join(a.Targets, ", "),
		"__CYCLE__", fmt.Sprint(a.Cycle), "__AREA_SKILLS__", a.AreaSkills, "__COMMANDS__", a.Commands,
		"__CANDIDATE_JSON__", candidateJSON, "__PACKET_JSON__", packetJSON, "__BASELINE__", a.BaselineSummary,
		"__PHASE_EVIDENCE__", jsonOf(a.PhaseEvidence),
		"__REVIEW_JSON__", jsonOf(review), "__DIFF__", a.Diff, "__RATIONALE__", defaultString(a.Rationale, "(none given)"),
		"__DEAD_PROVIDER__", a.DeadProvider, "__LIVE_PROVIDER__", a.LiveProvider, "__FAILURE_CLASS__", a.FailureClass,
		"__STATE_JSON__", a.StateJSON, "__JOURNAL_TAIL__", a.JournalTail, "__GIT_LOG__", a.GitLog,
	}
	if a.Provider == "codex" {
		s = strings.ReplaceAll(s, "/sharpen-", "$sharpen-")
		// The shared templates describe Claude's Read/Glob/Grep tools. Codex
		// exposes repository reads through a shell even in read-only sandbox
		// mode. Without this adaptation, a Codex audit reports no candidates
		// because it believes repository inspection is impossible.
		s = strings.ReplaceAll(s,
			"You have Read, Glob and Grep. You have no shell and no write access; this is",
			"You have a read-only shell tool for repository inspection. Use only file reads, `rg`, `rg --files`, and read-only `git` queries. Do not edit files, run project scripts, or access the network; this is")
		s = strings.ReplaceAll(s, "path with your Read tool", "path with read-only shell commands")
		s = strings.ReplaceAll(s, "with Read, Glob and Grep", "with read-only shell file searches")
		s = strings.ReplaceAll(s, "Everything else you do with Edit and Write as", "Everything else you do with the available file-editing tool as")
	}
	s = strings.NewReplacer(replacements...).Replace(s)
	if a.Incremental && a.Committed > 0 {
		s = strings.ReplaceAll(s, "landed 1 commit(s)", fmt.Sprintf("landed %d commit(s)", a.Committed))
	}
	// Conditional lists are expanded without changing the surrounding prose.
	done := "none"
	if len(a.SeenDone) > 0 {
		done = strings.Join(a.SeenDone, ", ")
	}
	s = strings.ReplaceAll(s, "__DONE__", done)
	skipped := "none"
	if len(a.SeenSkipped) > 0 {
		parts := make([]string, 0, len(a.SeenSkipped))
		for _, x := range a.SeenSkipped {
			parts = append(parts, x.Fingerprint+" ["+x.Reason+"]")
		}
		skipped = strings.Join(parts, ", ")
	}
	s = strings.ReplaceAll(s, "__SKIPPED_FP__ [__SKIPPED_REASON__]", skipped)
	if len(a.Violated) > 0 {
		s = strings.ReplaceAll(s, "__VIOLATED__", strings.Join(a.Violated, ", "))
	} else {
		s = strings.ReplaceAll(s, "paths a previous attempt tried to modify and must not touch: __VIOLATED__\n", "")
	}
	if a.Packet != nil {
		files := asStrings(a.Packet["characterization_files"])
		var rows []string
		for _, f := range files {
			rows = append(rows, "    "+f)
		}
		if len(rows) == 0 {
			rows = []string{"    (none named — that is itself a blocker; emit FAIL)"}
		}
		s = strings.ReplaceAll(s, "    __CHARFILE__", strings.Join(rows, "\n"))
		stops := asStrings(a.Packet["stop_conditions"])
		rows = nil
		for _, stop := range stops {
			rows = append(rows, "  - "+stop)
		}
		if len(rows) == 0 {
			rows = []string{"  - (none stated; treat any need to touch a file outside the allowlist as one)"}
		}
		s = strings.ReplaceAll(s, "  - __STOP__", strings.Join(rows, "\n"))
	}
	return s, nil
}
func defaultString(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
func RepairPrompt(nonce, errors, priorText string) string {
	if nonce == "" {
		nonce = NewNonce()
	}
	if priorText == "" {
		priorText = "(empty)"
	}
	b, _ := fs.ReadFile(promptFiles, "templates/repair.txt")
	return strings.NewReplacer("__NONCE__", nonce, "__ERRORS__", errors, "__PRIOR_TEXT__", priorText).Replace(string(b))
}
