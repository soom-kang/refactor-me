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
	"sort"
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
	_, err := InstallWithReport(repoArg, executable)
	return err
}

// InstallWithReport returns legacy library paths retained because their contents
// cannot be identified as files copied by the v0.8.8-beta.1 Node installer.
func InstallWithReport(repoArg, executable string) ([]string, error) {
	repo, err := RepoRoot(repoArg)
	if err != nil {
		return nil, err
	}
	if !samePath(repo, repoArg) {
		return nil, fmt.Errorf("install target must be repository root: %s", repo)
	}
	source, err := os.Open(executable)
	if err != nil {
		return nil, err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("source executable is not a regular file")
	}
	dest := filepath.Join(repo, ".refactor")
	binDir := filepath.Join(dest, "bin")
	binary := filepath.Join(binDir, "refactor-me")
	markerPath := filepath.Join(binDir, markerName)
	if err := checkDirectoryNotSymlink(dest); err != nil {
		return nil, err
	}
	if err := checkDirectoryNotSymlink(binDir); err != nil {
		return nil, err
	}
	oldKind, err := ownedBinary(binary, markerPath)
	if err != nil {
		return nil, err
	}
	if oldKind == "node" {
		for _, directory := range []string{filepath.Join(dest, "lib"), filepath.Join(dest, "lib", "src"), filepath.Join(dest, "lib", "bin")} {
			if err := checkDirectoryNotSymlink(directory); err != nil {
				return nil, err
			}
		}
	}
	legacy := filepath.Join(binDir, "refactorloop")
	if err := checkLegacyShim(legacy); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return nil, err
	}
	staged, err := os.CreateTemp(binDir, ".refactor-me-stage-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(staged.Name())
	h := sha256.New()
	if _, err = io.Copy(io.MultiWriter(staged, h), source); err != nil {
		staged.Close()
		return nil, err
	}
	if err = staged.Chmod(0755); err != nil {
		staged.Close()
		return nil, err
	}
	if err = staged.Sync(); err != nil {
		staged.Close()
		return nil, err
	}
	if err = staged.Close(); err != nil {
		return nil, err
	}
	marker := installMarker{Format: 1, Tool: "refactor-me-go", SHA256: hex.EncodeToString(h.Sum(nil))}
	markerData, _ := json.MarshalIndent(marker, "", "  ")
	markerData = append(markerData, '\n')
	stagedMarker, err := os.CreateTemp(binDir, ".refactor-marker-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(stagedMarker.Name())
	if _, err = stagedMarker.Write(markerData); err != nil {
		stagedMarker.Close()
		return nil, err
	}
	if err = stagedMarker.Sync(); err != nil {
		stagedMarker.Close()
		return nil, err
	}
	if err = stagedMarker.Close(); err != nil {
		return nil, err
	}
	// Keep a byte-for-byte backup until the new executable and marker are both in place.
	backup := filepath.Join(binDir, ".refactor-me-previous")
	if _, err = os.Lstat(backup); err == nil {
		return nil, fmt.Errorf("cannot install: backup path already exists: %s", backup)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if oldKind != "" {
		if err = os.Rename(binary, backup); err != nil {
			return nil, err
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
		return nil, restore(err)
	}
	if err = os.Rename(stagedMarker.Name(), markerPath); err != nil {
		return nil, restore(err)
	}
	if oldKind != "" {
		if err = os.Remove(backup); err != nil {
			return nil, fmt.Errorf("installed, but previous binary backup remains at %s: %w", backup, err)
		}
	}
	var retained []string
	if oldKind == "node" {
		retained = removeOwnedNodeRuntime(dest)
	}
	if err := removeRecognizedLegacy(legacy); err != nil {
		return retained, err
	}
	if err := writeDefaultsIfMissing(dest); err != nil {
		return retained, err
	}
	return retained, nil
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

// These hashes are the bytes copied by tool/install.mjs at tag v0.8.8-beta.1.
// The Node installer did not record per-file ownership, so other versions and
// modified files must remain in place even when the entry shim is recognizable.
var knownNodeFiles = map[string]string{
	"LICENSE":                 "2ff221ea907baefbb5770248bcc6b0ce6d4adc299bd6ed01ed34d1b8cc67b9d1",
	"bin/refactor-me.mjs":     "c3cbaa405c5c36aee456a69abb5ff799dcf7e9b4d976025e2440a9dd330a9ef1",
	"src/code-comparison.mjs": "4097357bb473f77ca724cf4b73b681c0024b91642f1cc21dc48495a57f53fdef",
	"src/config.mjs":          "9e6e31aece36bf27437fbed19b2fd229978e1098737069f79d23e47ef6a4f244",
	"src/doctor.mjs":          "8170e3b6fc5887c0d9030b28c9ebe464ec68f8bf2cec398a37d81f525f6ab59e",
	"src/gate.mjs":            "c7c3fe6f3908aad71800abd763529394fb8c71ebabb2155fc0e988defa1cebbd",
	"src/git.mjs":             "a44d515d95c8ea598a78560de23dbdc9b789274a969fb2b6d4eba1bd9e1f0896",
	"src/log.mjs":             "5cf0cbcef61cff259bf573e624cad1d4094961306bda9974da4f6a15ec480660",
	"src/loop.mjs":            "97388dba045541623e31d2644a975e5e128831cd67120e2aebeca146b8a67ea2",
	"src/prompts.mjs":         "489b6957dd2c5922c69c7901161e1458f33cb3c697d0a4b5560ea9ff26820ad0",
	"src/provider.mjs":        "4304ab9d72b21ebe52de694d6fe92cb1c7bce3df50ac7aff8034f693de326f4c",
	"src/report.mjs":          "b26950c0f599c797f7afaee1f88e0a2d6373b35679438cade265caa24ebda8ea",
	"src/schemas.mjs":         "3f3f5e1a5318bead94436e6f91dc8544042ea4c772eec8e104c09c9cd5e68db1",
	"src/scope.mjs":           "de906a0c73c237a0ee34e55bc46e03ffba20c0a4e821106c36a63bac70a2f510",
	"src/state.mjs":           "69a3356a3099a0d1fcee8320ebea9fdb8214dffdb0e037ab240c56bef684c3b1",
	"src/validate.mjs":        "537b43c953fb0af67df0b63987488fcd4a8e160ad432c8fb8db5f623fc5e4d73",
	"src/version.mjs":         "7a050e5f9ea2d66a67dcdcdcc06d7ce25d6e7fdf866b73957b9274ee5ae246f8",
}

func removeOwnedNodeRuntime(dest string) []string {
	lib := filepath.Join(dest, "lib")
	for name, expected := range knownNodeFiles {
		path := filepath.Join(lib, filepath.FromSlash(name))
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		actual, err := fileHash(path)
		if err == nil && actual == expected {
			// A removal failure leaves the path in the retained-path report below.
			_ = os.Remove(path)
		}
	}
	removeEmpty(filepath.Join(lib, "src"))
	removeEmpty(filepath.Join(lib, "bin"))
	removeEmpty(lib)

	var retained []string
	_ = filepath.WalkDir(lib, func(path string, entry os.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil || !entry.IsDir() {
			retained = append(retained, path)
		}
		return nil
	})
	sort.Strings(retained)
	return retained
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
