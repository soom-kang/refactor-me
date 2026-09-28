package surface

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Config retains unknown user keys for forward compatibility with Node configs.
type Config map[string]any

const defaultConfigJSON = `{
  "schema_version": 1,
  "workspace": {"branch_prefix":"refactor/auto-","worktree_parent":"","keep_worktree":true},
  "agents": {
    "primary":"codex","fallback":"claude",
    "codex":{"enabled":true,"bin":"codex","model":"","timeout_sec":1800,"effort_by_phase":{}},
    "claude":{"enabled":true,"bin":"claude","model":"","timeout_sec":1800,"max_budget_usd":0,"effort_by_phase":{}}
  },
  "policy": {
    "allowed_risks":["L0_LOW","L1_MODERATE","L2_HIGH"],"unknown_risk":"set_aside",
    "auto_characterization":true,"cross_provider_review":true,"max_cycles":25,"max_commits":20,
    "max_consecutive_failures":3,"max_attempts_per_fingerprint":2,"empty_audits_to_stop":2,
    "max_files_per_candidate":8,"max_changed_lines":600,"max_audit_candidates":8,
    "max_wall_clock_min":180,"cooldown_minutes":20,"extra_forbidden_globs":[]
  },
  "verification":{"locked":false,"commands":[]}
}`

func DefaultConfig() Config {
	var cfg Config
	if err := json.Unmarshal([]byte(defaultConfigJSON), &cfg); err != nil {
		panic(err)
	}
	return cfg
}

// LoadConfig copies Node's one-level merge for workspace, policy and verification,
// and its two-level merge for agents. Unknown fields remain available to callers.
func LoadConfig(repo string) (Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(filepath.Join(repo, ".refactor", "config.json"))
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, err
	}
	var user Config
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("config.json: %w", err)
	}
	if user == nil {
		return cfg, nil
	}
	for key, value := range user {
		switch key {
		case "workspace", "policy", "verification":
			cfg[key] = mergeObject(object(cfg[key]), object(value))
		case "agents":
			agents := mergeObject(object(cfg[key]), object(value))
			for _, name := range []string{"codex", "claude"} {
				agents[name] = mergeObject(object(object(cfg[key])[name]), object(object(value)[name]))
			}
			cfg[key] = agents
		default:
			cfg[key] = value
		}
	}
	return cfg, nil
}

func object(value any) map[string]any {
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func mergeObject(base, user map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(user))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range user {
		out[key] = value
	}
	return out
}

func ConfigString(cfg Config, keys ...string) string {
	var value any = map[string]any(cfg)
	for _, key := range keys {
		value = object(value)[key]
	}
	text, _ := value.(string)
	return text
}
