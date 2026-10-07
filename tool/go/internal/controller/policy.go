package controller

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type policy struct {
	AllowedRisks              []string
	UnknownRisk               string
	AutoCharacterization      bool
	CrossProviderReview       bool
	MaxCycles                 int
	MaxCommits                int
	MaxConsecutiveFailures    int
	MaxAttemptsPerFingerprint int
	EmptyAuditsToStop         int
	MaxFilesPerCandidate      int
	MaxAuditCandidates        int
	MaxWallClockMin           int
	CooldownMinutes           int
}

func readPolicy(config map[string]any) (policy, error) {
	m := obj(config["policy"])
	p := policy{
		AllowedRisks: stringsOf(m["allowed_risks"]), UnknownRisk: str(m["unknown_risk"]),
		AutoCharacterization: boolValue(m["auto_characterization"], true),
		CrossProviderReview:  boolValue(m["cross_provider_review"], true),
		MaxCycles:            integer(m["max_cycles"], 25), MaxCommits: integer(m["max_commits"], 20),
		MaxConsecutiveFailures:    integer(m["max_consecutive_failures"], 3),
		MaxAttemptsPerFingerprint: integer(m["max_attempts_per_fingerprint"], 2),
		EmptyAuditsToStop:         integer(m["empty_audits_to_stop"], 2),
		MaxFilesPerCandidate:      integer(m["max_files_per_candidate"], 8),
		MaxAuditCandidates:        integer(m["max_audit_candidates"], 8),
		MaxWallClockMin:           integer(m["max_wall_clock_min"], 180),
		CooldownMinutes:           integer(m["cooldown_minutes"], 20),
	}
	if p.UnknownRisk != "set_aside" && p.UnknownRisk != "deep_check" {
		return p, fmt.Errorf("policy.unknown_risk must be set_aside or deep_check")
	}
	if p.MaxCycles < 1 || p.MaxCommits < 1 || p.EmptyAuditsToStop < 1 || p.MaxWallClockMin < 1 {
		return p, fmt.Errorf("policy limits must be positive")
	}
	if p.MaxWallClockMin > int(time.Duration(1<<63-1)/time.Minute) {
		return p, fmt.Errorf("policy.max_wall_clock_min exceeds the supported duration")
	}
	return p, nil
}

func obj(value any) map[string]any {
	m, _ := value.(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}

func str(value any) string {
	s, _ := value.(string)
	return s
}

func stringsOf(value any) []string {
	if items, ok := value.([]string); ok {
		return slices.Clone(items)
	}
	items, _ := value.([]any)
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func mapsOf(value any) []map[string]any {
	items, _ := value.([]any)
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func integer(value any, fallback int) int {
	switch n := value.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		return fallback
	}
}

func boolValue(value any, fallback bool) bool {
	if b, ok := value.(bool); ok {
		return b
	}
	return fallback
}

func fingerprint(candidate map[string]any) string {
	paths := stringsOf(candidate["related_files"])
	normal := make([]string, 0, len(paths))
	for _, path := range paths {
		normal = append(normal, strings.TrimSuffix(strings.TrimPrefix(filepath.ToSlash(path), "./"), "/"))
	}
	slices.Sort(normal)
	normal = slices.Compact(normal)
	raw := str(candidate["category"]) + "\x00" + strings.Join(normal, ",") + "\x00" + strings.ToLower(str(candidate["primary_symbol"]))
	h := sha1.Sum([]byte(raw))
	return hex.EncodeToString(h[:])[:12]
}

type candidateDecision struct {
	Candidate map[string]any
	Reason    string
	Detail    string
}

func rank(candidates []map[string]any, targets []string, p policy, seen map[string]bool, attempts map[string]int) (eligible []map[string]any, rejected []candidateDecision) {
	for _, c := range candidates {
		fp := fingerprint(c)
		c["fp"] = fp
		reject := func(reason, detail string) { rejected = append(rejected, candidateDecision{c, reason, detail}) }
		if !touchesTarget(stringsOf(c["related_files"]), targets) {
			reject("OUT_OF_TARGET", "no related file under target")
			continue
		}
		risk := str(c["risk_level"])
		if risk == "UNKNOWN" {
			if !(p.UnknownRisk == "deep_check" && c["readiness"] == "NEEDS_EVIDENCE") {
				reject("RISK_UNKNOWN", str(c["problem"]))
				continue
			}
		} else if !slices.Contains(p.AllowedRisks, risk) {
			reject("RISK_EXCLUDED", risk)
			continue
		}
		if c["readiness"] == "REJECT" {
			reject("MODEL_REJECTED", str(c["problem"]))
			continue
		}
		if seen[fp] {
			reject("ALREADY_SEEN", "")
			continue
		}
		if attempts[fp] >= p.MaxAttemptsPerFingerprint {
			reject("ATTEMPTS_EXHAUSTED", "")
			continue
		}
		if integer(c["estimated_file_count"], 1) > p.MaxFilesPerCandidate {
			reject("TOO_LARGE", "")
			continue
		}
		eligible = append(eligible, c)
	}
	order := map[string]int{"L0_LOW": 0, "L1_MODERATE": 1, "L2_HIGH": 2, "L3_CRITICAL": 3, "UNKNOWN": 4}
	slices.SortFunc(eligible, func(a, b map[string]any) int {
		if d := order[str(a["risk_level"])] - order[str(b["risk_level"])]; d != 0 {
			return d
		}
		ar, br := 1, 1
		if a["readiness"] == "READY" {
			ar = 0
		}
		if b["readiness"] == "READY" {
			br = 0
		}
		if d := ar - br; d != 0 {
			return d
		}
		if d := integer(a["estimated_file_count"], 9) - integer(b["estimated_file_count"], 9); d != 0 {
			return d
		}
		return strings.Compare(str(a["candidate_id"]), str(b["candidate_id"]))
	})
	return eligible, rejected
}

func touchesTarget(paths, targets []string) bool {
	if len(targets) == 0 {
		return true
	}
	for _, path := range paths {
		for _, target := range targets {
			if path == target || strings.HasPrefix(path, target+"/") {
				return true
			}
		}
	}
	return false
}
