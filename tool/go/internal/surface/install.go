package surface

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const markerName = ".refactor-me.install.json"

type installMarker struct {
	Format int    `json:"format"`
	Tool   string `json:"tool"`
	SHA256 string `json:"sha256"`
}

var nodeShimPattern = regexp.MustCompile(`(?s)^#!/bin/sh\n# refactor-me [^\n]+ — installed \d{4}-\d{2}-\d{2} from [^\n]+\nexec node "\$\(dirname "\$0"\)/\.\./lib/bin/refactor-me\.mjs" "\$@"\n$`)
var legacyShimPattern = regexp.MustCompile(`(?s)^#!/bin/sh\n# refactorloop [^\n]+ — installed \d{4}-\d{2}-\d{2} from [^\n]+\nexec node "\$\(dirname "\$0"\)/\.\./lib/bin/refactorloop\.mjs" "\$@"\n$`)

// Install replaces only a recognized Node shim or a binary matching our marker.
// The source executable must be a complete, readable distribution before any
// destination file is touched.
func Install(repoArg, executable string) error {
	repo, err := RepoRoot(repoArg)
	if err != nil {
		return err
	}
	if !samePath(repo, repoArg) {
		return fmt.Errorf("install target must be repository root: %s", repo)
	}
	source, err := os.Open(executable)
	if err != nil {
		return err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("source executable is not a regular file")
	}
	dest := filepath.Join(repo, ".refactor")
	binDir := filepath.Join(dest, "bin")
	binary := filepath.Join(binDir, "refactor-me")
	markerPath := filepath.Join(binDir, markerName)
	if err := checkDirectoryNotSymlink(dest); err != nil {
		return err
	}
	if err := checkDirectoryNotSymlink(binDir); err != nil {
		return err
	}
	oldKind, err := ownedBinary(binary, markerPath)
	if err != nil {
		return err
	}
	if oldKind == "node" {
		for _, directory := range []string{filepath.Join(dest, "lib"), filepath.Join(dest, "lib", "src"), filepath.Join(dest, "lib", "bin")} {
			if err := checkDirectoryNotSymlink(directory); err != nil {
				return err
			}
		}
	}
	legacy := filepath.Join(binDir, "refactorloop")
	if err := checkLegacyShim(legacy); err != nil {
		return err
	}
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return err
	}
	staged, err := os.CreateTemp(binDir, ".refactor-me-stage-*")
	if err != nil {
		return err
	}
	defer os.Remove(staged.Name())
	h := sha256.New()
	if _, err = io.Copy(io.MultiWriter(staged, h), source); err != nil {
		staged.Close()
		return err
	}
	if err = staged.Chmod(0755); err != nil {
		staged.Close()
		return err
	}
	if err = staged.Sync(); err != nil {
		staged.Close()
		return err
	}
	if err = staged.Close(); err != nil {
		return err
	}
	marker := installMarker{Format: 1, Tool: "refactor-me-go", SHA256: hex.EncodeToString(h.Sum(nil))}
	markerData, _ := json.MarshalIndent(marker, "", "  ")
	markerData = append(markerData, '\n')
	stagedMarker, err := os.CreateTemp(binDir, ".refactor-marker-*")
	if err != nil {
		return err
	}
	defer os.Remove(stagedMarker.Name())
	if _, err = stagedMarker.Write(markerData); err != nil {
		stagedMarker.Close()
		return err
	}
	if err = stagedMarker.Sync(); err != nil {
		stagedMarker.Close()
		return err
	}
	if err = stagedMarker.Close(); err != nil {
		return err
	}
	// Keep a byte-for-byte backup until the new executable and marker are both in place.
	backup := filepath.Join(binDir, ".refactor-me-previous")
	if _, err = os.Lstat(backup); err == nil {
		return fmt.Errorf("cannot install: backup path already exists: %s", backup)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if oldKind != "" {
		if err = os.Rename(binary, backup); err != nil {
			return err
		}
	}
	restore := func(e error) error {
		os.Remove(binary)
		if oldKind != "" {
			if backErr := os.Rename(backup, binary); backErr != nil {
				return fmt.Errorf("install failed: %v; restore failed: %w", e, backErr)
			}
		}
		return e
	}
	if err = os.Rename(staged.Name(), binary); err != nil {
		return restore(err)
	}
	if err = os.Rename(stagedMarker.Name(), markerPath); err != nil {
		return restore(err)
	}
	if oldKind != "" {
		if err = os.Remove(backup); err != nil {
			return fmt.Errorf("installed, but previous binary backup remains at %s: %w", backup, err)
		}
	}
	if oldKind == "node" {
		removeOwnedNodeRuntime(dest)
	}
	if err := removeRecognizedLegacy(legacy); err != nil {
		return err
	}
	if err := writeDefaultsIfMissing(dest); err != nil {
		return err
	}
	return nil
}

// Uninstall removes only an unchanged Go binary identified by its checksum.
func Uninstall(repoArg string) error {
	repo, err := RepoRoot(repoArg)
	if err != nil {
		return err
	}
	if !samePath(repo, repoArg) {
		return fmt.Errorf("uninstall target must be repository root: %s", repo)
	}
	binDir := filepath.Join(repo, ".refactor", "bin")
	if err := checkDirectoryNotSymlink(filepath.Join(repo, ".refactor")); err != nil {
		return err
	}
	if err := checkDirectoryNotSymlink(binDir); err != nil {
		return err
	}
	binary := filepath.Join(binDir, "refactor-me")
	marker := filepath.Join(binDir, markerName)
	kind, err := ownedBinary(binary, marker)
	if err != nil {
		return err
	}
	if kind != "go" {
		return errors.New("no owned Go installation found")
	}
	if err = os.Remove(binary); err != nil {
		return err
	}
	if err = os.Remove(marker); err != nil {
		return err
	}
	return nil
}

func samePath(repo, arg string) bool {
	abs, err := filepath.Abs(arg)
	if err != nil {
		return false
	}
	real, err := filepath.EvalSymlinks(abs)
	return err == nil && real == repo
}

func checkDirectoryNotSymlink(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("installation directory is not a regular directory: %s", path)
	}
	return nil
}

func ownedBinary(binary, markerPath string) (string, error) {
	info, err := os.Lstat(binary)
	if errors.Is(err, os.ErrNotExist) {
		if _, markerErr := os.Lstat(markerPath); markerErr == nil {
			return "", fmt.Errorf("unrecognized orphan install marker at %s", markerPath)
		} else if !errors.Is(markerErr, os.ErrNotExist) {
			return "", markerErr
		}
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("unrecognized file at %s", binary)
	}
	if markerInfo, markerErr := os.Lstat(markerPath); markerErr == nil && !markerInfo.Mode().IsRegular() {
		return "", fmt.Errorf("unrecognized install marker at %s", markerPath)
	} else if markerErr != nil && !errors.Is(markerErr, os.ErrNotExist) {
		return "", markerErr
	}
	if data, markerErr := os.ReadFile(markerPath); markerErr == nil {
		var marker installMarker
		if json.Unmarshal(data, &marker) != nil || marker.Format != 1 || marker.Tool != "refactor-me-go" {
			return "", fmt.Errorf("unrecognized install marker at %s", markerPath)
		}
		actual, e := fileHash(binary)
		if e != nil {
			return "", e
		}
		if !strings.EqualFold(actual, marker.SHA256) {
			return "", fmt.Errorf("installed binary differs from ownership marker: %s", binary)
		}
		return "go", nil
	} else if !errors.Is(markerErr, os.ErrNotExist) {
		return "", markerErr
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		return "", err
	}
	if nodeShimPattern.Match(data) {
		return "node", nil
	}
	return "", fmt.Errorf("unrecognized file at %s", binary)
}

func fileHash(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func checkLegacyShim(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unrecognized file at %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !legacyShimPattern.Match(data) {
		return fmt.Errorf("unrecognized file at %s", path)
	}
	return nil
}

func removeRecognizedLegacy(path string) error {
	if err := checkLegacyShim(path); err != nil {
		return err
	}
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return os.Remove(path)
}

func removeOwnedNodeRuntime(dest string) {
	// Each path is from the install.mjs source-copy manifest. Unknown files remain.
	for _, name := range []string{"config.mjs", "prompts.mjs", "code-comparison.mjs", "doctor.mjs", "scope.mjs", "git.mjs", "gate.mjs", "report.mjs", "log.mjs", "provider.mjs", "loop.mjs", "schemas.mjs", "validate.mjs", "state.mjs", "version.mjs"} {
		removeRegular(filepath.Join(dest, "lib", "src", name))
	}
	removeRegular(filepath.Join(dest, "lib", "bin", "refactor-me.mjs"))
	removeRegular(filepath.Join(dest, "lib", "bin", "refactorloop.mjs"))
	removeEmpty(filepath.Join(dest, "lib", "src"))
	removeEmpty(filepath.Join(dest, "lib", "bin"))
	removeRegular(filepath.Join(dest, "lib", "LICENSE"))
	removeEmpty(filepath.Join(dest, "lib"))
}

func removeRegular(path string) {
	if info, e := os.Lstat(path); e == nil && info.Mode().IsRegular() {
		_ = os.Remove(path)
	}
}
func removeEmpty(path string) {
	entries, e := os.ReadDir(path)
	if e == nil && len(entries) == 0 {
		_ = os.Remove(path)
	}
}

func writeDefaultsIfMissing(dest string) error {
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}
	for path, content := range map[string]string{filepath.Join(dest, "config.json"): defaultConfigJSON + "\n", filepath.Join(dest, ".gitignore"): "*\n"} {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return err
		}
		if _, err = file.WriteString(content); err != nil {
			file.Close()
			return err
		}
		if err = file.Close(); err != nil {
			return err
		}
	}
	return nil
}
