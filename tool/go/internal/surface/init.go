package surface

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Init creates optional project settings without replacing existing files.
func Init(repo string) error {
	dir := filepath.Join(repo, ".refactor")
	for _, path := range []string{dir, filepath.Join(dir, "runs")} {
		if err := checkDirectory(path); err != nil {
			return err
		}
	}
	configPath := filepath.Join(dir, "config.json")
	ignorePath := filepath.Join(dir, ".gitignore")
	for _, path := range []string{configPath, ignorePath} {
		if err := checkRegularFile(path); err != nil {
			return err
		}
	}
	if _, err := LoadConfig(repo); err != nil {
		return err
	}
	ignore, err := os.ReadFile(ignorePath)
	if err == nil && !bytes.Equal(bytes.TrimSpace(ignore), []byte("*")) {
		return errors.New("existing .refactor/.gitignore is not owned by refactor-me")
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read ignore file: %w", err)
	}
	data, err := json.MarshalIndent(DefaultConfig(), "", "  ")
	if err != nil {
		return fmt.Errorf("encode default config: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "runs"), 0o755); err != nil {
		return fmt.Errorf("create project state: %w", err)
	}
	if err := createOptionalFile(ignorePath, []byte("*\n")); err != nil {
		return err
	}
	return createOptionalFile(configPath, append(data, '\n'))
}

func checkDirectory(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a regular directory", path)
	}
	return nil
}

func checkRegularFile(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	return nil
}

func createOptionalFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
