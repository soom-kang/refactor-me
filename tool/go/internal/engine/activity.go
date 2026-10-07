package engine

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ActivityEvents extracts an allowlisted operation and an optional local path.
// Provider prose, search patterns, commands, arguments and tool output are never
// copied to progress events. Tool-call accounting remains independent.
func ActivityEvents(provider, phase, cwd string, event map[string]any) []ProviderEvent {
	if provider != "claude" && provider != "codex" {
		return nil
	}
	makeEvent := func(action, status, path string, allowMissing bool) ProviderEvent {
		return ProviderEvent{Kind: "activity", Provider: provider, Phase: phase, Action: action, Status: status,
			Path: activityPath(cwd, path, allowMissing)}
	}
	var events []ProviderEvent
	if provider == "claude" && (event["type"] == "assistant" || event["type"] == "user") {
		message, _ := event["message"].(map[string]any)
		parts, _ := message["content"].([]any)
		for _, raw := range parts {
			part, _ := raw.(map[string]any)
			if part["type"] == "tool_result" {
				status := "completed"
				if part["is_error"] == true {
					status = "failed"
				}
				events = append(events, makeEvent("tool", status, "", false))
				continue
			}
			if part["type"] != "tool_use" {
				continue
			}
			input, _ := part["input"].(map[string]any)
			path, _ := input["file_path"].(string)
			action, allowMissing := "tool", false
			switch part["name"] {
			case "Read":
				action = "read"
			case "Edit", "MultiEdit":
				action = "edit"
			case "Write":
				action, allowMissing = "write", true
			case "Grep", "Glob":
				action = "search"
				path, _ = input["path"].(string)
			}
			events = append(events, makeEvent(action, "started", path, allowMissing))
		}
		return events
	}
	if provider != "codex" || (event["type"] != "item.started" && event["type"] != "item.completed") {
		return nil
	}
	item, _ := event["item"].(map[string]any)
	status := "started"
	if event["type"] == "item.completed" {
		status = "completed"
	}
	if item["status"] == "failed" {
		status = "failed"
	}
	switch item["type"] {
	case "command_execution":
		if exit, ok := item["exit_code"].(float64); ok && exit != 0 {
			status = "failed"
		}
		return []ProviderEvent{makeEvent("command", status, "", false)}
	case "file_change":
		changes, _ := item["changes"].([]any)
		for _, raw := range changes {
			change, _ := raw.(map[string]any)
			path, _ := change["path"].(string)
			action, allowMissing := "edit", false
			switch change["kind"] {
			case "add":
				action, allowMissing = "write", true
			case "delete":
				action, allowMissing = "delete", true
			}
			events = append(events, makeEvent(action, status, path, allowMissing))
		}
		if len(events) == 0 {
			events = append(events, makeEvent("edit", status, "", false))
		}
		return events
	case "mcp_tool_call":
		return []ProviderEvent{makeEvent("tool", status, "", false)}
	case "web_search":
		return []ProviderEvent{makeEvent("search", status, "", false)}
	}
	return nil
}

func activityPath(cwd, raw string, allowMissing bool) string {
	if cwd == "" || raw == "" || len(raw) > 4096 || !utf8.ValidString(raw) {
		return ""
	}
	for _, r := range raw {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ""
		}
	}
	lexicalRoot, err := filepath.Abs(cwd)
	if err != nil {
		return ""
	}
	root, err := filepath.EvalSymlinks(lexicalRoot)
	if err != nil {
		return ""
	}
	abs := raw
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, abs)
	} else if lexicalRoot != root {
		// macOS exposes /var and /tmp as aliases of /private/var and
		// /private/tmp. Preserve containment through the selected root alias.
		if rel, err := filepath.Rel(lexicalRoot, abs); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			abs = filepath.Join(root, rel)
		}
	}
	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || sensitiveActivityPath(rel) {
		return ""
	}
	// Existing symlinks, including an ancestor of a new/deleted file, must still
	// resolve inside the worktree. Never print a path into a credential directory.
	probe := abs
	for {
		real, err := filepath.EvalSymlinks(probe)
		if err == nil {
			realRel, err := filepath.Rel(root, real)
			if err != nil || realRel == ".." || strings.HasPrefix(realRel, ".."+string(filepath.Separator)) || sensitiveActivityPath(realRel) {
				return ""
			}
			return filepath.ToSlash(rel)
		}
		if !allowMissing || !os.IsNotExist(err) || probe == root {
			return ""
		}
		probe = filepath.Dir(probe)
	}
}

func sensitiveActivityPath(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		part = strings.ToLower(part)
		if part == ".git" || part == ".refactor" || part == ".aws" || part == ".ssh" || part == ".kube" || part == ".codex" || part == ".claude" ||
			part == ".env" || strings.HasPrefix(part, ".env.") || strings.Contains(part, "credential") || strings.Contains(part, "secret") ||
			strings.Contains(part, "kubeconfig") || strings.Contains(part, "keychain") || part == "id_rsa" || part == "id_ed25519" {
			return true
		}
		switch filepath.Ext(part) {
		case ".pem", ".key", ".p12", ".pfx":
			return true
		}
	}
	return false
}
