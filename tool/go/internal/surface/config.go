package surface

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ConfigSchemaVersion identifies supported project settings.
const ConfigSchemaVersion = 2

// Config stores settings, including unknown extension keys.
type Config map[string]any

const defaultConfigJSON = `{
  "schema_version": 2,
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

// LoadConfig merges versioned settings with defaults while retaining extension keys.
func LoadConfig(repo string) (Config, error) {
	cfg := DefaultConfig()
	if err := checkDirectory(filepath.Join(repo, ".refactor")); err != nil {
		return nil, err
	}
	if err := checkRegularFile(filepath.Join(repo, ".refactor", "config.json")); err != nil {
		return nil, err
	}
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
	if user["schema_version"] != float64(ConfigSchemaVersion) {
		return nil, fmt.Errorf("config.json: unsupported schema_version %v; expected %d", user["schema_version"], ConfigSchemaVersion)
	}
	// Validate the original numbers before float64 decoding can round a fraction
	// into an integer or an integer outside the supported range.
	var exact Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&exact); err != nil {
		return nil, fmt.Errorf("config.json: %w", err)
	}
	if err := ValidateConfig(exact); err != nil {
		return nil, fmt.Errorf("config.json: %w", err)
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

// ValidateConfig checks known settings when present; omitted defaults and
// unknown extension keys are left to the caller. It does not modify config.
func ValidateConfig(cfg Config) error {
	if value, present := cfg["schema_version"]; present {
		if _, err := configInteger(value, "schema_version", ConfigSchemaVersion, ConfigSchemaVersion); err != nil {
			return err
		}
	}
	for _, section := range []string{"workspace", "agents", "policy", "verification"} {
		if _, err := configObject(map[string]any(cfg), section, section); err != nil {
			return err
		}
	}
	workspace := object(cfg["workspace"])
	for _, key := range []string{"branch_prefix", "worktree_parent"} {
		if err := configType(workspace, key, "workspace."+key, "string"); err != nil {
			return err
		}
	}
	if err := configType(workspace, "keep_worktree", "workspace.keep_worktree", "boolean"); err != nil {
		return err
	}
	agents := object(cfg["agents"])
	for _, key := range []string{"primary", "fallback"} {
		if err := configType(agents, key, "agents."+key, "string"); err != nil {
			return err
		}
	}
	for _, name := range []string{"codex", "claude"} {
		path := "agents." + name
		agent, err := configObject(agents, name, path)
		if err != nil {
			return err
		}
		for _, key := range []string{"bin", "model", "effort"} {
			if err := configType(agent, key, path+"."+key, "string"); err != nil {
				return err
			}
		}
		if err := configType(agent, "enabled", path+".enabled", "boolean"); err != nil {
			return err
		}
		if value, present := agent["timeout_sec"]; present {
			if _, err := configInteger(value, path+".timeout_sec", 0, math.MaxInt64/int64(time.Second)); err != nil {
				return err
			}
		}
		if value, present := agent["max_budget_usd"]; present {
			number, ok := configNumber(value)
			if !ok || number < 0 {
				return fmt.Errorf("%s.max_budget_usd must be a finite nonnegative number", path)
			}
		}
		phases, err := configObject(agent, "effort_by_phase", path+".effort_by_phase")
		if err != nil {
			return err
		}
		for phase := range phases {
			if err := configType(phases, phase, path+".effort_by_phase."+phase, "string"); err != nil {
				return err
			}
		}
	}
	policy := object(cfg["policy"])
	if err := configType(policy, "unknown_risk", "policy.unknown_risk", "string"); err != nil {
		return err
	}
	if value, present := policy["unknown_risk"]; present && value != "set_aside" && value != "deep_check" {
		return errors.New("policy.unknown_risk must be set_aside or deep_check")
	}
	for _, key := range []string{"auto_characterization", "cross_provider_review"} {
		if err := configType(policy, key, "policy."+key, "boolean"); err != nil {
			return err
		}
	}
	for _, key := range []string{"allowed_risks", "extra_forbidden_globs"} {
		if err := configType(policy, key, "policy."+key, "string array"); err != nil {
			return err
		}
	}
	for _, key := range []string{"max_cycles", "max_commits", "max_consecutive_failures", "max_attempts_per_fingerprint", "empty_audits_to_stop", "max_files_per_candidate", "max_changed_lines", "max_audit_candidates", "max_wall_clock_min", "cooldown_minutes"} {
		minimum, maximum := int64(0), int64(^uint(0)>>1)
		switch key {
		case "max_cycles", "max_commits", "empty_audits_to_stop":
			minimum = 1
		case "max_wall_clock_min":
			minimum, maximum = 1, maxRuntimeMinutes
		case "cooldown_minutes":
			maximum = maxRuntimeMinutes
		}
		if value, present := policy[key]; present {
			if _, err := configInteger(value, "policy."+key, minimum, maximum); err != nil {
				return err
			}
		}
	}
	verification := object(cfg["verification"])
	if err := configType(verification, "locked", "verification.locked", "boolean"); err != nil {
		return err
	}
	return configType(verification, "commands", "verification.commands", "array")
}

func configObject(parent map[string]any, key, path string) (map[string]any, error) {
	value, present := parent[key]
	if !present {
		return nil, nil
	}
	if value, ok := value.(map[string]any); ok && value != nil {
		return value, nil
	}
	return nil, fmt.Errorf("%s must be an object", path)
}

func configType(parent map[string]any, key, path, kind string) error {
	value, present := parent[key]
	if !present {
		return nil
	}
	valid := false
	switch kind {
	case "string":
		_, valid = value.(string)
	case "boolean":
		_, valid = value.(bool)
	case "array", "string array":
		if items, ok := value.([]any); ok {
			valid = items != nil
			if kind == "string array" {
				for index, item := range items {
					if _, ok := item.(string); !ok {
						return fmt.Errorf("%s[%d] must be a string", path, index)
					}
				}
			}
		} else if items, ok := value.([]string); ok {
			valid = items != nil
		}
	}
	if !valid {
		return fmt.Errorf("%s must be a %s", path, kind)
	}
	return nil
}

func configNumber(value any) (float64, bool) {
	var number float64
	switch value := value.(type) {
	case int:
		number = float64(value)
	case float64:
		number = value
	case json.Number:
		var err error
		number, err = value.Float64()
		if err != nil {
			return 0, false
		}
		mantissa := strings.SplitN(strings.ToLower(string(value)), "e", 2)[0]
		if number == 0 && strings.Trim(mantissa, "-+.0") != "" {
			return 0, false
		}
	default:
		return 0, false
	}
	return number, !math.IsNaN(number) && !math.IsInf(number, 0)
}

func configInteger(value any, path string, minimum, maximum int64) (int, error) {
	fail := func() (int, error) {
		return 0, fmt.Errorf("%s must be a whole number from %d to %d without precision loss", path, minimum, maximum)
	}
	if number, ok := value.(int); ok {
		if int64(number) < minimum || int64(number) > maximum {
			return fail()
		}
		return number, nil
	}
	number, ok := configNumber(value)
	if !ok || math.Trunc(number) != number || number < float64(minimum) || number >= math.Exp2(63) {
		return fail()
	}
	if raw, ok := value.(json.Number); ok {
		if number == 0 {
			return 0, nil
		}
		text := string(raw)
		if index := strings.IndexAny(text, "eE"); index >= 0 {
			exponent, err := strconv.ParseInt(text[index+1:], 10, 64)
			// Bound exact-parser allocation by the input length. A nonzero
			// integer in int64 cannot require a more extreme decimal shift.
			if err != nil || exponent < -int64(index) || exponent > int64(index)+19 {
				return fail()
			}
		}
		exact, ok := new(big.Rat).SetString(string(raw))
		if !ok || !exact.IsInt() || !exact.Num().IsInt64() || new(big.Rat).SetFloat64(number).Cmp(exact) != 0 {
			return fail()
		}
	}
	if int64(number) < minimum || int64(number) > maximum || int64(number) > int64(^uint(0)>>1) {
		return fail()
	}
	return int(number), nil
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
