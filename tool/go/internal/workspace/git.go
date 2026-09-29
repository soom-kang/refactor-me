package workspace

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
)

var ownedWorktrees sync.Map

// ErrUnsafe marks a failure that must stop publication and preserve the worktree.
var ErrUnsafe = errors.New("HALTED_UNSAFE")

// Git executes an argv vector without a shell. A failed command includes stderr.
func Git(cwd string, args ...string) (string, error) {
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func RepoRoot(start string) (string, error) {
	out, err := Git(start, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(strings.TrimSpace(out))
}

func HeadOID(cwd string) (string, error) {
	out, err := Git(cwd, "rev-parse", "--verify", "HEAD")
	return strings.TrimSpace(out), err
}

func CurrentBranch(cwd string) (string, error) {
	out, err := Git(cwd, "symbolic-ref", "--quiet", "--short", "HEAD")
	return strings.TrimSpace(out), err
}

func StatusPorcelain(cwd string) (string, error) {
	return Git(cwd, "status", "--porcelain", "--untracked-files=normal")
}

func SourceFingerprint(cwd string) (string, error) {
	head, err := HeadOID(cwd)
	if err != nil {
		return "", err
	}
	status, err := StatusPorcelain(cwd)
	if err != nil {
		return "", err
	}
	h := sha1.Sum([]byte(status))
	return head + ":" + hex.EncodeToString(h[:]), nil
}

func WorktreePath(root, id, parent string) string {
	if parent == "" {
		home, _ := os.UserHomeDir()
		parent = filepath.Join(home, ".cache", "refactor-me")
	}
	h := sha1.Sum([]byte(root))
	return filepath.Join(parent, filepath.Base(root)+"-"+hex.EncodeToString(h[:4]), id)
}

func worktreeOutsideRoot(root, wt string) error {
	rootAbs, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	wtAbs, err := filepath.Abs(wt)
	if err != nil {
		return err
	}
	// Resolve a symlinked parent even before the worktree itself exists.
	parentReal, err := filepath.EvalSymlinks(filepath.Dir(wtAbs))
	if err == nil {
		wtAbs = filepath.Join(parentReal, filepath.Base(wtAbs))
	}
	rel, err := filepath.Rel(rootAbs, wtAbs)
	if err != nil {
		return err
	}
	if rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..") {
		return fmt.Errorf("worktree must be outside source repository")
	}
	return nil
}

func WorktreeAdd(root, wt, baseOID string) error {
	if err := worktreeOutsideRoot(root, wt); err != nil {
		return err
	}
	if _, err := os.Stat(wt); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("worktree path already exists: %s", wt)
	}
	if err := os.MkdirAll(filepath.Dir(wt), 0o755); err != nil {
		return err
	}
	if err := worktreeOutsideRoot(root, wt); err != nil {
		return err
	}
	if _, err := Git(root, "worktree", "add", "--detach", wt, baseOID); err != nil {
		return err
	}
	ownedWorktrees.Store(filepath.Clean(wt), filepath.Clean(root))
	return nil
}

func checkOwnedWorktree(root, wt string) error {
	owner, ok := ownedWorktrees.Load(filepath.Clean(wt))
	if !ok || owner != filepath.Clean(root) {
		return fmt.Errorf("worktree is not owned by this process: %s", wt)
	}
	actual, err := RepoRoot(wt)
	if err != nil {
		return err
	}
	actualRoot, err := filepath.EvalSymlinks(wt)
	if err != nil {
		return err
	}
	if actual != actualRoot {
		return fmt.Errorf("worktree root mismatch: %s", wt)
	}
	if err := checkWorktreeRegistration(root, wt); err != nil {
		return err
	}
	return worktreeOutsideRoot(root, wt)
}

func gitCommonDir(cwd string) (string, error) {
	out, err := Git(cwd, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(out)
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	return filepath.EvalSymlinks(path)
}

func checkWorktreeRegistration(root, wt string) error {
	sourceCommon, err := gitCommonDir(root)
	if err != nil {
		return err
	}
	worktreeCommon, err := gitCommonDir(wt)
	if err != nil {
		return err
	}
	if sourceCommon != worktreeCommon {
		return fmt.Errorf("worktree does not share source Git database")
	}
	registered, err := Git(root, "worktree", "list", "--porcelain")
	if err != nil {
		return err
	}
	wtReal, err := filepath.EvalSymlinks(wt)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(registered, "\n") {
		if !strings.HasPrefix(line, "worktree ") {
			continue
		}
		registeredReal, resolveErr := filepath.EvalSymlinks(strings.TrimPrefix(line, "worktree "))
		if resolveErr == nil && registeredReal == wtReal {
			return nil
		}
	}
	return fmt.Errorf("worktree is not registered with source repository")
}

// WorktreeRemove never forces removal; a dirty worktree remains for inspection.
func WorktreeRemove(root, wt string) error {
	if err := checkOwnedWorktree(root, wt); err != nil {
		return err
	}
	if _, err := Git(root, "worktree", "remove", wt); err != nil {
		return err
	}
	ownedWorktrees.Delete(filepath.Clean(wt))
	return nil
}

// WorktreeRemoveRecorded removes a completed worktree from an earlier process.
// The saved run identity, Git registration and clean status must all agree.
func WorktreeRemoveRecorded(root, wt, runID string) error {
	if runID == "" || filepath.Base(filepath.Clean(wt)) != runID {
		return fmt.Errorf("recorded worktree does not match run id")
	}
	if err := worktreeOutsideRoot(root, wt); err != nil {
		return err
	}
	var saved struct {
		Schema   int    `json:"schema"`
		RunID    string `json:"runId"`
		RepoRoot string `json:"repoRoot"`
		Worktree string `json:"worktree"`
		Terminal struct {
			Status string `json:"status"`
		} `json:"terminal"`
	}
	file := filepath.Join(root, ".refactor", "runs", runID, "state.json")
	if err := ReadJSON(file, &saved); err != nil {
		return fmt.Errorf("read worktree record: %w", err)
	}
	if saved.Schema != StateSchema {
		return fmt.Errorf("unsupported state schema %d; expected %d", saved.Schema, StateSchema)
	}
	if saved.RunID != runID || filepath.Clean(saved.RepoRoot) != filepath.Clean(root) || filepath.Clean(saved.Worktree) != filepath.Clean(wt) {
		return fmt.Errorf("worktree record identity mismatch")
	}
	if saved.Terminal.Status != "DONE" && saved.Terminal.Status != "NO_CHANGES" {
		return fmt.Errorf("run is not safely complete: %s", saved.Terminal.Status)
	}
	if err := checkWorktreeRegistration(root, wt); err != nil {
		return err
	}
	actual, err := RepoRoot(wt)
	if err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(wt)
	if err != nil {
		return err
	}
	if actual != resolved {
		return fmt.Errorf("recorded worktree root mismatch")
	}
	status, err := StatusPorcelain(wt)
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) != "" {
		return fmt.Errorf("recorded worktree is dirty; preserving it")
	}
	_, err = Git(root, "worktree", "remove", wt)
	return err
}

func nulPaths(out string) []string {
	items := strings.Split(out, "\x00")
	var paths []string
	for _, item := range items {
		if item != "" {
			paths = append(paths, item)
		}
	}
	return paths
}

func ChangedPaths(wt string) ([]string, error) {
	tracked, err := Git(wt, "diff", "--name-only", "-z", "--no-renames", "HEAD")
	if err != nil {
		return nil, err
	}
	untracked, err := Git(wt, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, p := range append(nulPaths(tracked), nulPaths(untracked)...) {
		seen[p] = true
	}
	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	return paths, nil
}

func DeletedPaths(wt string) ([]string, error) {
	out, err := Git(wt, "diff", "--name-only", "-z", "--diff-filter=D", "HEAD")
	return nulPaths(out), err
}

type DiffStats struct {
	Insertions, Deletions int
	Binary                []string
	PerFile               map[string]FileStat
}

func Numstat(wt, ref string) (DiffStats, error) {
	if ref == "" {
		ref = "HEAD"
	}
	out, err := Git(wt, "diff", "--numstat", "-z", "--no-renames", ref)
	if err != nil {
		return DiffStats{}, err
	}
	result := DiffStats{PerFile: map[string]FileStat{}}
	for _, line := range nulPaths(out) {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		p := parts[2]
		if parts[0] == "-" || parts[1] == "-" {
			result.Binary = append(result.Binary, p)
			result.PerFile[p] = FileStat{Binary: true}
			continue
		}
		ins, _ := strconv.Atoi(parts[0])
		del, _ := strconv.Atoi(parts[1])
		result.Insertions += ins
		result.Deletions += del
		result.PerFile[p] = FileStat{Insertions: ins, Deletions: del}
	}
	return result, nil
}

func FileLineCount(root, rel string) *int { return countLines(filepath.Join(root, rel)) }
func DiffPatch(wt, ref string) (string, error) {
	if ref == "" {
		ref = "HEAD"
	}
	return Git(wt, "diff", ref)
}

func gitDiffNoIndex(wt, file string) (string, error) {
	cmd := exec.Command("git", "diff", "--no-index", "--", "/dev/null", file)
	cmd.Dir = wt
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			return "", fmt.Errorf("git diff --no-index: %w: %s", err, stderr.String())
		}
	}
	return stdout.String(), nil
}

// DiffForReview includes untracked text files and names omitted binary files.
func DiffForReview(wt, ref string) (string, error) {
	tracked, err := DiffPatch(wt, ref)
	if err != nil {
		return "", err
	}
	untracked, err := Git(wt, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString(tracked)
	var binary []string
	for _, rel := range nulPaths(untracked) {
		if _, ok := binaryBytes(filepath.Join(wt, rel)); ok {
			binary = append(binary, rel)
			continue
		}
		piece, err := gitDiffNoIndex(wt, rel)
		if err != nil {
			return "", err
		}
		b.WriteString(piece)
	}
	if len(binary) > 0 {
		b.WriteString(fmt.Sprintf("\n# (%d untracked binary file(s) omitted as build output: %s)\n", len(binary), strings.Join(binary, ", ")))
	}
	return b.String(), nil
}

func binaryBytes(path string) (int64, bool) {
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() || st.Size() == 0 {
		return 0, false
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer f.Close()
	buf := make([]byte, 8192)
	n, _ := f.Read(buf)
	return st.Size(), bytes.IndexByte(buf[:n], 0) >= 0
}

type SweptArtifact struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

// SweepBuildArtifacts removes only untracked binaries or files under known
// build output directories of a worktree owned by this process.
func SweepBuildArtifacts(root, wt string, outputDirs []string) ([]SweptArtifact, error) {
	if err := checkOwnedWorktree(root, wt); err != nil {
		return nil, err
	}
	out, err := Git(wt, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	var swept []SweptArtifact
	for _, rel := range nulPaths(out) {
		abs := filepath.Join(wt, rel)
		bytes, binary := binaryBytes(abs)
		inOutput := false
		for _, dir := range outputDirs {
			dir = strings.TrimSuffix(filepath.ToSlash(dir), "/")
			if rel == dir || strings.HasPrefix(rel, dir+"/") {
				inOutput = true
				break
			}
		}
		if !binary && !inOutput {
			continue
		}
		if !binary {
			st, err := os.Lstat(abs)
			if err != nil {
				return swept, err
			}
			bytes = st.Size()
		}
		if err := os.Remove(abs); err != nil {
			return swept, err
		}
		swept = append(swept, SweptArtifact{Path: rel, Bytes: bytes})
	}
	return swept, nil
}

func StageExact(wt string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	deleted, err := Git(wt, "diff", "--cached", "--name-only", "-z", "--diff-filter=D", "HEAD")
	if err != nil {
		return err
	}
	wasDeleted := map[string]bool{}
	for _, p := range nulPaths(deleted) {
		wasDeleted[p] = true
	}
	args := []string{"add", "-A", "--"}
	for _, p := range paths {
		if filepath.IsAbs(p) || p == ".." || strings.HasPrefix(filepath.Clean(p), ".."+string(filepath.Separator)) {
			return fmt.Errorf("invalid stage path: %s", p)
		}
		if !wasDeleted[p] {
			args = append(args, p)
		}
	}
	if len(args) == 3 {
		return nil
	}
	_, err = Git(wt, args...)
	return err
}

func WriteTree(wt string) (string, error) {
	out, err := Git(wt, "write-tree")
	return strings.TrimSpace(out), err
}

// CommitApproved runs hooks and refuses publication when hooks change the approved tree.
func CommitApproved(wt, messageFile, approvedTree string) (string, error) {
	before, err := WriteTree(wt)
	if err != nil {
		return "", err
	}
	if before != approvedTree {
		return "", fmt.Errorf("staged tree differs from approved tree")
	}
	_, err = Git(wt, "-c", "user.name=refactor-me", "-c", "user.email=refactor-me@local", "commit", "-F", messageFile)
	if err != nil {
		return "", fmt.Errorf("%w: commit or hook failed: %v", ErrUnsafe, err)
	}
	oid, err := HeadOID(wt)
	if err != nil {
		return "", err
	}
	committed, err := Git(wt, "rev-parse", oid+"^{tree}")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(committed) != approvedTree {
		return "", fmt.Errorf("%w: hook changed committed tree", ErrUnsafe)
	}
	status, err := StatusPorcelain(wt)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(status) != "" {
		return "", fmt.Errorf("%w: hook left worktree changes", ErrUnsafe)
	}
	return oid, nil
}

// PublishCAS updates a branch only if its previous value is exactly as expected.
func PublishCAS(root, ref, newOID, prevOID string) error {
	if !strings.HasPrefix(ref, "refs/heads/") {
		return fmt.Errorf("publication requires a branch ref")
	}
	_, err := Git(root, "update-ref", ref, newOID, prevOID)
	return err
}

func RefExists(root, ref string) bool {
	_, err := Git(root, "rev-parse", "--verify", "--quiet", ref)
	return err == nil
}

type RollbackResult struct {
	Clean         bool
	Head, Residue string
}

// Rollback is destructive only within a worktree created by WorktreeAdd in this process.
func Rollback(root, wt, preOID string) (RollbackResult, error) {
	if err := checkOwnedWorktree(root, wt); err != nil {
		return RollbackResult{}, err
	}
	if _, err := Git(wt, "reset", "--hard", preOID); err != nil {
		return RollbackResult{}, err
	}
	if _, err := Git(wt, "clean", "-ffd"); err != nil {
		return RollbackResult{}, err
	}
	head, err := HeadOID(wt)
	if err != nil {
		return RollbackResult{}, err
	}
	status, err := StatusPorcelain(wt)
	if err != nil {
		return RollbackResult{}, err
	}
	return RollbackResult{Clean: strings.TrimSpace(status) == "" && head == preOID, Head: head, Residue: strings.TrimSpace(status)}, nil
}

func CheckoutDetached(wt, oid string) error {
	_, err := Git(wt, "checkout", "--detach", oid)
	return err
}

func LogOneline(root, revision string, limit int) (string, error) {
	return Git(root, "log", "--oneline", "-"+strconv.Itoa(limit), revision)
}
