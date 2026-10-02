package surface

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReportRequestedSettingsAreOptionalAndNotExecutionEvidence(t *testing.T) {
	report := map[string]any{
		"schemaVersion": 3, "runId": "settings", "status": "NO_CHANGES",
		"providerSettings": map[string]any{
			"codex":  map[string]any{"requestedModel": "custom|model", "cliEffort": "xhigh"},
			"claude": map[string]any{"requestedModel": "configured-model"},
		},
		"usage": map[string]any{"totals": map[string]any{"processes": 1, "calls": 1}, "byPhase": map[string]any{"doctor": map[string]any{"calls": 1}}},
	}
	for _, language := range []string{"en", "ko"} {
		data, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		text, err := RenderReport(data, language)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"custom\\|model", "configured-model", "xhigh"} {
			if !strings.Contains(text, want) {
				t.Fatal(language, "missing requested setting", want, text)
			}
		}
		if language == "en" && (!strings.Contains(text, "Default effort") || !strings.Contains(text, "do not verify which model")) {
			t.Fatal("settings or default table claim execution evidence", text)
		}
		if language == "ko" && (!strings.Contains(text, "기본 추론 수준") || !strings.Contains(text, "증거는 아닙니다")) {
			t.Fatal("Korean settings or default table claim execution evidence", text)
		}
	}
	delete(report, "providerSettings")
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	text, err := RenderReport(data, "en")
	if err != nil || strings.Contains(text, "Requested provider settings") {
		t.Fatal("old schema-3 report failed or invented settings", text, err)
	}
}
