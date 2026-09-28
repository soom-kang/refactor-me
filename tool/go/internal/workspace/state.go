package workspace

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const StateSchema = 1

func Fingerprint(kind string, paths []string, symbol string) string {
	set := map[string]bool{}
	for _, p := range paths {
		set[strings.TrimRight(strings.TrimPrefix(p, "./"), "/")] = true
	}
	clean := make([]string, 0, len(set))
	for p := range set {
		clean = append(clean, p)
	}
	slices.Sort(clean)
	h := sha1.Sum([]byte(kind + "\x00" + strings.Join(clean, ",") + "\x00" + strings.ToLower(symbol)))
	return hex.EncodeToString(h[:])[:12]
}

func RunID(now time.Time) (string, error) {
	b := make([]byte, 2)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return now.Format("20060102-1504") + "-" + hex.EncodeToString(b), nil
}

// WriteJSONAtomic fsyncs the temporary file and its parent before returning.
func WriteJSONAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	buf, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	buf = append(buf, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".refactor-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(buf); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func ReadJSON(path string, dest any) error {
	buf, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(buf, dest); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func EnsureRefactorDir(root string) (string, error) {
	dir := filepath.Join(root, ".refactor")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ignore := filepath.Join(dir, ".gitignore")
	buf, err := os.ReadFile(ignore)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(ignore, []byte("*\n"), 0o644); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	} else if strings.TrimSpace(string(buf)) != "*" {
		return "", fmt.Errorf("existing .refactor/.gitignore is not owned by refactor-me")
	}
	return dir, nil
}

func RunDirFor(root, id string) (string, error) {
	base, err := EnsureRefactorDir(root)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "runs", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func CycleDirFor(runDir string, cycle int, category, fingerprint string) (string, error) {
	dir := filepath.Join(runDir, "cycles", fmt.Sprintf("%02d-%s-%s", cycle, category, fingerprint))
	return dir, os.MkdirAll(filepath.Join(dir, "provider"), 0o755)
}

func AuditDirFor(runDir string, cycle int) (string, error) {
	dir := filepath.Join(runDir, "audits", fmt.Sprintf("%02d", cycle))
	return dir, os.MkdirAll(filepath.Join(dir, "provider"), 0o755)
}

type SeenState struct {
	Done     []string           `json:"done"`
	Skipped  []SkippedCandidate `json:"skipped"`
	Attempts map[string]int     `json:"attempts"`
}
type SkippedCandidate struct {
	FP     string `json:"fp"`
	Reason string `json:"reason"`
	Detail any    `json:"detail"`
	At     string `json:"at"`
}
type Counters struct {
	Cycles              int `json:"cycles"`
	Commits             int `json:"commits"`
	ConsecutiveFailures int `json:"consecutiveFailures"`
	EmptyAudits         int `json:"emptyAudits"`
	AuditsNoProposals   int `json:"auditsNoProposals"`
	AuditsAllFiltered   int `json:"auditsAllFiltered"`
	Skipped             int `json:"skipped"`
	Violations          int `json:"violations"`
}
type RunState struct {
	Schema         int            `json:"schema"`
	ToolVersion    string         `json:"toolVersion"`
	RunID          string         `json:"runId"`
	State          string         `json:"state"`
	Cycle          int            `json:"cycle"`
	RepoRoot       string         `json:"repoRoot"`
	Worktree       string         `json:"worktree"`
	Targets        []string       `json:"targets"`
	BaseOID        string         `json:"baseOid"`
	BaseBranch     string         `json:"baseBranch"`
	BranchName     string         `json:"branchName"`
	PublishedOID   *string        `json:"publishedOid"`
	StartedAt      string         `json:"startedAt"`
	UpdatedAt      string         `json:"updatedAt"`
	Providers      map[string]any `json:"providers"`
	ProviderOrder  []string       `json:"providerOrder"`
	ActiveProvider *string        `json:"activeProvider"`
	Counters       Counters       `json:"counters"`
	Usage          map[string]any `json:"usage"`
	Seen           SeenState      `json:"seen"`
	TreeHashes     []string       `json:"treeHashes"`
	Commits        []any          `json:"commits"`
	ActivePacket   any            `json:"activePacket"`
	Terminal       any            `json:"terminal"`
}

func NewState(id, root, wt, baseOID, baseBranch, branchName string, providers map[string]any, order, targets []string, version string) *RunState {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if order == nil {
		for k := range providers {
			order = append(order, k)
		}
		slices.Sort(order)
	}
	return &RunState{Schema: StateSchema, ToolVersion: version, RunID: id, State: "INIT", RepoRoot: root,
		Worktree: wt, BaseOID: baseOID, BaseBranch: baseBranch, BranchName: branchName, Targets: targets,
		StartedAt: now, UpdatedAt: now, Providers: providers, ProviderOrder: order,
		Usage: map[string]any{"byPhase": map[string]any{}}, Seen: SeenState{Done: []string{}, Skipped: []SkippedCandidate{}, Attempts: map[string]int{}},
		TreeHashes: []string{}, Commits: []any{}}
}

type Store struct {
	File  string
	State *RunState
}

func NewStore(runDir string) *Store { return &Store{File: filepath.Join(runDir, "state.json")} }
func (s *Store) Load() error {
	var st RunState
	if err := ReadJSON(s.File, &st); err != nil {
		return err
	}
	s.State = &st
	return nil
}
func (s *Store) Save() error {
	if s.State == nil {
		return fmt.Errorf("state is nil")
	}
	s.State.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return WriteJSONAtomic(s.File, s.State)
}
func (s *Store) Init(st *RunState) error      { s.State = st; return s.Save() }
func (s *Store) Transition(next string) error { s.State.State = next; return s.Save() }
func (s *RunState) IsSeen(fp string) bool {
	if slices.Contains(s.Seen.Done, fp) {
		return true
	}
	for _, x := range s.Seen.Skipped {
		if x.FP == fp {
			return true
		}
	}
	return false
}
func (s *RunState) NoteAttempt(fp string) int {
	if s.Seen.Attempts == nil {
		s.Seen.Attempts = map[string]int{}
	}
	s.Seen.Attempts[fp]++
	return s.Seen.Attempts[fp]
}
func (s *RunState) MarkDone(fp string) {
	if !slices.Contains(s.Seen.Done, fp) {
		s.Seen.Done = append(s.Seen.Done, fp)
	}
}
func (s *RunState) MarkSkipped(fp, reason string, detail any) {
	if s.IsSeen(fp) {
		return
	}
	s.Seen.Skipped = append(s.Seen.Skipped, SkippedCandidate{FP: fp, Reason: reason, Detail: detail, At: time.Now().UTC().Format(time.RFC3339Nano)})
	s.Counters.Skipped++
}

type Lock struct {
	File string
	held bool
}
type LockHolder struct {
	PID       int    `json:"pid"`
	StartedAt string `json:"startedAt"`
	Hostname  string `json:"hostname"`
}

func NewLock(file string) *Lock { return &Lock{File: file} }

// Acquire returns the current holder when a live or unidentifiable lock exists.
func (l *Lock) Acquire() (*LockHolder, error) {
	if err := os.MkdirAll(filepath.Dir(l.File), 0o755); err != nil {
		return nil, err
	}
	host, _ := os.Hostname()
	mine := LockHolder{PID: os.Getpid(), StartedAt: time.Now().UTC().Format(time.RFC3339Nano), Hostname: host}
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(l.File, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			encErr := json.NewEncoder(f).Encode(mine)
			closeErr := f.Close()
			if encErr != nil {
				return nil, encErr
			}
			if closeErr != nil {
				return nil, closeErr
			}
			l.held = true
			return nil, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		var holder LockHolder
		if readErr := ReadJSON(l.File, &holder); readErr != nil {
			return &holder, nil
		}
		if holder.Hostname == host && holder.PID > 0 && !pidAlive(holder.PID) {
			if err := os.Remove(l.File); err != nil {
				return nil, err
			}
			continue
		}
		return &holder, nil
	}
	return nil, fmt.Errorf("failed to acquire lock after stale recovery")
}
func pidAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
func (l *Lock) Release() error {
	if !l.held {
		return nil
	}
	var holder LockHolder
	if err := ReadJSON(l.File, &holder); err != nil {
		return err
	}
	if holder.PID == os.Getpid() {
		if err := os.Remove(l.File); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	l.held = false
	return nil
}

func ReadLastRun(root string) (map[string]any, error) {
	var result map[string]any
	err := ReadJSON(filepath.Join(root, ".refactor", "last-run.json"), &result)
	return result, err
}

func ParseRunNumber(value any) int { n, _ := strconv.Atoi(fmt.Sprint(value)); return n }
