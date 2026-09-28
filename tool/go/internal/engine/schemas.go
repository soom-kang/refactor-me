package engine

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
)

//go:embed schemas.json
var schemaJSON []byte

// Schema returns a fresh copy of the strict provider contract for a phase.
// The names match tool/src/schemas.mjs, including "deepcheck" without an underscore.
func Schema(phase string) map[string]any {
	if phase == "deep_check" {
		phase = "deepcheck"
	}
	var all map[string]map[string]any
	if err := json.Unmarshal(schemaJSON, &all); err != nil {
		panic("embedded schemas: " + err.Error())
	}
	return all[phase]
}

// Validate checks the subset of JSON Schema used by the provider contracts.
// An empty slice means the value satisfies the schema.
func Validate(schema map[string]any, value any) []string {
	return validateAt(schema, value, "$")
}

func validateAt(schema map[string]any, value any, at string) []string {
	types := schemaTypes(schema["type"])
	actual := jsonType(value)
	ok := false
	for _, t := range types {
		if t == actual || (t == "number" && actual == "integer") {
			ok = true
		}
	}
	if !ok {
		return []string{fmt.Sprintf("%s: expected %s, got %s", at, strings.Join(types, "|"), actual)}
	}
	var errs []string
	if options, ok := schema["enum"].([]any); ok {
		found := false
		labels := make([]string, 0, len(options))
		for _, option := range options {
			labels = append(labels, fmt.Sprint(option))
			if reflect.DeepEqual(value, option) {
				found = true
			}
		}
		if !found {
			b, _ := json.Marshal(value)
			errs = append(errs, fmt.Sprintf("%s: %s is not one of %s", at, b, strings.Join(labels, "|")))
		}
	}
	if actual == "object" {
		obj, _ := value.(map[string]any)
		props, _ := schema["properties"].(map[string]any)
		if required, ok := schema["required"].([]any); ok {
			for _, r := range required {
				key := fmt.Sprint(r)
				if _, exists := obj[key]; !exists {
					errs = append(errs, at+"."+key+": required property is missing")
				}
			}
		}
		if schema["additionalProperties"] == false {
			keys := make([]string, 0, len(obj))
			for k := range obj {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				if _, exists := props[k]; !exists {
					errs = append(errs, at+"."+k+": unexpected property")
				}
			}
		}
		keys := make([]string, 0, len(props))
		for k := range props {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if child, exists := obj[k]; exists {
				if sub, ok := props[k].(map[string]any); ok {
					errs = append(errs, validateAt(sub, child, at+"."+k)...)
				}
			}
		}
	}
	if actual == "array" {
		if sub, ok := schema["items"].(map[string]any); ok {
			for i, item := range value.([]any) {
				errs = append(errs, validateAt(sub, item, fmt.Sprintf("%s[%d]", at, i))...)
			}
		}
	}
	return errs
}

func schemaTypes(value any) []string {
	switch v := value.(type) {
	case string:
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			out = append(out, fmt.Sprint(x))
		}
		return out
	default:
		return []string{"undefined"}
	}
}

func jsonType(value any) string {
	switch v := value.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "boolean"
	case json.Number:
		f, err := v.Float64()
		if err == nil && math.Trunc(f) == f {
			return "integer"
		}
		return "number"
	case float64:
		if math.Trunc(v) == v {
			return "integer"
		}
		return "number"
	case int, int64, uint64:
		return "integer"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	default:
		return "undefined"
	}
}

// EnforceExecutionVerdict applies the non-model rules to an execution result.
func EnforceExecutionVerdict(result, packet map[string]any) (string, []string) {
	var reasons []string
	bad := 0
	for _, h := range asObjects(result["hunks"]) {
		if h["classification"] == "UNEXPLAINED" || h["classification"] == "CONTRACT_CHANGING" {
			bad++
		}
	}
	if bad > 0 {
		reasons = append(reasons, fmt.Sprintf("%d unexplained or contract-changing hunk(s)", bad))
	}
	if result["scope_expansion_required"] == true {
		reasons = append(reasons, "scope expansion required")
	}
	if packet != nil && packet["category"] != "LARGE_COMPONENT_SPLIT" {
		allow := map[string]bool{}
		for _, f := range asStrings(packet["allowlist"]) {
			allow[f] = true
		}
		var outside []string
		for _, f := range append(asStrings(result["changed_files"]), asStrings(result["deleted_files"])...) {
			if !allow[f] {
				outside = append(outside, f)
			}
		}
		if len(outside) > 0 {
			reasons = append(reasons, "claims changes outside the allowlist: "+strings.Join(outside, ", "))
		}
	}
	if len(reasons) > 0 {
		return "FAIL", reasons
	}
	return fmt.Sprint(result["verdict"]), nil
}

func EnforcePreflightVerdict(result map[string]any) (string, []string) {
	var reasons []string
	if result["falsification_result"] != "FALSIFIED" {
		reasons = append(reasons, "falsification "+fmt.Sprint(result["falsification_result"]))
	}
	if n := len(asObjects(result["blocking_reasons"])); n > 0 {
		reasons = append(reasons, fmt.Sprintf("%d blocking reason(s)", n))
	}
	if len(reasons) > 0 {
		return "BLOCKED", reasons
	}
	return fmt.Sprint(result["verdict"]), nil
}

func EnforceReviewVerdict(result map[string]any) (string, []string) {
	var reasons []string
	n := 0
	for _, f := range asObjects(result["findings"]) {
		if f["severity"] == "BLOCKER" {
			n++
		}
	}
	if n > 0 {
		reasons = append(reasons, fmt.Sprintf("%d BLOCKER finding(s)", n))
	}
	if result["behavior_preservation_assessment"] != "PRESERVED" {
		reasons = append(reasons, "behaviour assessment "+fmt.Sprint(result["behavior_preservation_assessment"]))
	}
	if len(reasons) > 0 {
		return "FAIL", reasons
	}
	return fmt.Sprint(result["verdict"]), nil
}

func EnforceCharacterizationVerdict(result map[string]any) (string, []string) {
	var reasons []string
	if result["production_source_changed"] == true {
		reasons = append(reasons, "production source was modified in a test-only slice")
	}
	if result["assertions_weakened"] == true {
		reasons = append(reasons, "existing assertions were weakened")
	}
	if len(reasons) > 0 {
		return "FAIL", reasons
	}
	return fmt.Sprint(result["verdict"]), nil
}

func asObjects(value any) []map[string]any {
	array, _ := value.([]any)
	out := make([]map[string]any, 0, len(array))
	for _, x := range array {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}
func asStrings(value any) []string {
	if direct, ok := value.([]string); ok {
		return append([]string(nil), direct...)
	}
	array, _ := value.([]any)
	out := make([]string, 0, len(array))
	for _, x := range array {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
