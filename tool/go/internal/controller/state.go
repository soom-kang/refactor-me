package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/soom-kang/refactor-me/tool/go/internal/engine"
)

const runSchema = 2

type counters struct {
	Cycles              int `json:"cycles"`
	Commits             int `json:"commits"`
	Violations          int `json:"violations"`
	ConsecutiveFailures int `json:"consecutiveFailures"`
	EmptyAudits         int `json:"emptyAudits"`
	AuditsNoProposals   int `json:"auditsNoProposals"`
	AuditsAllFiltered   int `json:"auditsAllFiltered"`
}

type providerStatus struct {
	Status        string              `json:"status"`
	Calls         int                 `json:"calls"`
	QuotaHits     int                 `json:"quotaHits"`
	LastError     string              `json:"lastError,omitempty"`
	CooldownUntil string              `json:"cooldownUntil,omitempty"`
	Usage         engine.UsageSummary `json:"usage"`
}

type skippedCandidate struct {
	FP     string         `json:"fp"`
	Reason string         `json:"reason"`
	Detail map[string]any `json:"detail,omitempty"`
}

type seenState struct {
	Done     []string           `json:"done"`
	Skipped  []skippedCandidate `json:"skipped"`
	Attempts map[string]int     `json:"attempts"`
}

type commitRecord struct {
	OID      string   `json:"oid"`
	FP       string   `json:"fp"`
	Category string   `json:"category"`
	Paths    []string `json:"paths"`
	Subject  string   `json:"subject"`
}

type terminal struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
	At     string `json:"at"`
}

type runState struct {
	Schema         int                        `json:"schema"`
	ToolVersion    string                     `json:"toolVersion"`
	RunID          string                     `json:"runId"`
	State          string                     `json:"state"`
	Cycle          int                        `json:"cycle"`
	RepoRoot       string                     `json:"repoRoot"`
	Worktree       string                     `json:"worktree"`
	Targets        []string                   `json:"targets"`
	BaseOID        string                     `json:"baseOid"`
	BaseBranch     string                     `json:"baseBranch"`
	BranchName     string                     `json:"branchName"`
	PublishedOID   string                     `json:"publishedOid,omitempty"`
	ProviderOrder  []string                   `json:"providerOrder"`
	ActiveProvider string                     `json:"activeProvider,omitempty"`
	Providers      map[string]*providerStatus `json:"providers"`
	Usage          engine.RingUsage           `json:"usage"`
	Counters       counters                   `json:"counters"`
	Seen           seenState                  `json:"seen"`
	Commits        []commitRecord             `json:"commits"`
	TreeHashes     []string                   `json:"treeHashes"`
	ActivePacket   map[string]any             `json:"activePacket,omitempty"`
	Terminal       *terminal                  `json:"terminal,omitempty"`
}

func newState(id, root, wt, base, branch, resultBranch string, targets, order []string) *runState {
	providers := map[string]*providerStatus{}
	for _, name := range []string{"codex", "claude"} {
		providers[name] = &providerStatus{Status: "READY"}
	}
	return &runState{
		Schema: runSchema, ToolVersion: "dev", RunID: id, State: "INIT", RepoRoot: root,
		Worktree: wt, Targets: append([]string{}, targets...), BaseOID: base, BaseBranch: branch,
		BranchName: resultBranch, ProviderOrder: order, Providers: providers,
		Usage:   engine.RingUsage{ByPhase: map[string]engine.UsageSummary{}},
		Seen:    seenState{Done: []string{}, Skipped: []skippedCandidate{}, Attempts: map[string]int{}},
		Commits: []commitRecord{}, TreeHashes: []string{},
	}
}

func (s *runState) seen(fp string) bool {
	for _, done := range s.Seen.Done {
		if done == fp {
			return true
		}
	}
	for _, skipped := range s.Seen.Skipped {
		if skipped.FP == fp {
			return true
		}
	}
	return false
}

func (s *runState) markSkipped(fp, reason, detail string, paths []string, violated bool) {
	s.Seen.Skipped = append(s.Seen.Skipped, skippedCandidate{FP: fp, Reason: reason, Detail: map[string]any{
		"detail": detail, "paths": paths, "violated": violated,
	}})
}

func (s *runState) finish(status, reason string) {
	s.Terminal = &terminal{Status: status, Reason: reason, At: time.Now().UTC().Format(time.RFC3339Nano)}
}

func writeJSONAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := os.CreateTemp(filepath.Dir(path), ".refactor-write-*")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return nil
}

func readJSONObject(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if value == nil {
		return nil, errors.New("JSON object required")
	}
	return value, nil
}

type halt struct{ Status, Reason string }

func (h halt) Error() string { return h.Reason }
