package surface

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReportEvidence(t *testing.T) {
	for status, titles := range map[string][2]string{"DONE": {"Completed", "완료"}, "NO_CHANGES": {"No changes", "변경 없음"}, "DONE_PARTIAL": {"Partially completed", "부분 완료"}, "ABORTED": {"Aborted", "중단"}, "HALTED_UNSAFE": {"Safety halt", "안전 정지"}} {
		report := map[string]any{"schemaVersion": 3, "runId": "r", "status": status, "reason": "no eligible candidates remain", "toolVersion": "dev", "worktree": "/evidence", "commits": []any{map[string]any{"oid": "a1b2c3d", "category": "DEAD_CODE", "subject": "s", "paths": []string{"src/a.go"}}}, "validation": map[string]any{"describe": "GREEN 3 / RED 1 of 4"}, "skipped": []any{map[string]any{"reason": "RISK_UNKNOWN", "detail": "Original evidence 31415"}, map[string]any{"reason": "NEW_CODE", "detail": "raw error"}}}
		b, _ := json.Marshal(report)
		for i, lang := range []string{"en", "ko"} {
			out, err := RenderReport(b, lang)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{titles[i], "dev", "a1b2c3d", "GREEN 3 / RED 1 of 4", "Original evidence 31415", "NEW_CODE", "raw error"} {
				if !strings.Contains(out, want) {
					t.Fatalf("%s %s lacks %s", status, lang, want)
				}
			}
			if strings.Contains(out, "증거로 보존한 worktree") != (lang == "ko" && status == "HALTED_UNSAFE") {
				t.Fatal("halt evidence notice")
			}
		}
		after, _ := json.Marshal(report)
		if !bytes.Equal(b, after) {
			t.Fatal("report mutated")
		}
	}
	for _, tc := range []struct {
		name           string
		cost           any
		missing, calls int
		en, ko         string
	}{
		{"unknown", nil, 2, 2, "Not reported", "미보고"}, {"partial", 1.41, 1, 2, "$1.41 or more", "$1.41 이상"}, {"full", 0.51, 0, 1, "$0.51", "$0.51"}, {"none", nil, 0, 0, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := json.Marshal(map[string]any{"schemaVersion": 3, "status": "DONE", "usage": map[string]any{"totals": map[string]any{"processes": tc.calls, "calls": tc.calls, "inputTokens": 456, "outputTokens": 123, "costUsd": tc.cost, "costMissing": tc.missing}}})
			for i, lang := range []string{"en", "ko"} {
				out, err := RenderReport(b, lang)
				if err != nil {
					t.Fatal(err)
				}
				want := []string{tc.en, tc.ko}[i]
				if want != "" && !strings.Contains(out, want) {
					t.Fatal(out)
				}
				if tc.calls == 0 && (strings.Contains(out, "## Cost and time") || strings.Contains(out, "## 비용과 시간")) {
					t.Fatal(out)
				}
				if strings.Contains(out, "$0.00") {
					t.Fatal(out)
				}
			}
		})
	}
	old, err := RenderReport([]byte(`{"schemaVersion":3,"status":"DONE"}`), "en")
	if err != nil || strings.Contains(old, "undefined") || !strings.Contains(old, "Not recorded") || !strings.Contains(old, "No code comparison was saved") {
		t.Fatal(old, err)
	}
	for _, lang := range []string{"en", "ko"} {
		out, err := RenderReport([]byte("{broken"), lang)
		if err == nil || out != "" {
			t.Fatal("corrupt report accepted")
		}
	}
	if _, err := RenderReport([]byte(`{}`), "fr"); err == nil {
		t.Fatal("unsupported language accepted")
	}
}

func TestChangesMarkdownEscapesAndKeepsFullDiff(t *testing.T) {
	patch := strings.Repeat("+unchanged evidence\n", 400) + "+````\n-final line\n"
	comparison := map[string]any{"baseCommit": strings.Repeat("a", 40), "resultCommit": strings.Repeat("b", 40), "status": "AVAILABLE",
		"totals": map[string]int{"files": 1, "insertions": 401, "deletions": 1, "binary": 0},
		"files":  []map[string]any{{"path": "[link](url)|`file`\n.txt", "status": "A", "insertions": 401, "deletions": 1, "oldMode": "000000", "newMode": "100644"}}}
	for _, language := range []string{"en", "ko"} {
		md := RenderChanges(comparison, patch, language)
		if !strings.Contains(md, patch) || !strings.Contains(md, "`````diff\n") || !strings.Contains(md, "&#91;link&#93;") || strings.Count(md, "- [ ]") != 1 || !strings.Contains(md, "+401 / -1") {
			t.Fatal("unsafe or incomplete Markdown", language)
		}
	}
}

func TestReportAccountedCostAndLimits(t *testing.T) {
	report := []byte(`{"schemaVersion":3,"status":"DONE_PARTIAL","costPolicy":"provider_reported_then_standard_estimate","runLimits":{"maxMinutes":30,"source":"cli","actualSeconds":1802,"exceeded":true},"usage":{"totals":{"processes":3,"calls":3,"costUsd":0.3,"estimatedCostUsd":0.15,"estimateMissing":1,"costEstimates":[{"provider":"claude","phase":"audit","requestedModel":"claude-sonnet-5-5","costUsd":0.15,"uncachedInputTokens":50000,"cachedInputTokens":50000,"cacheWriteInputTokens":10000,"cacheWrite1hInputTokens":1000,"outputTokens":1350,"rates":{"inputUsdPerMillion":2,"cachedInputUsdPerMillion":0.2,"cacheWriteUsdPerMillion":2.5,"cacheWrite1hUsdPerMillion":4,"outputUsdPerMillion":10,"sourceUrl":"https://artificialanalysis.ai/","supplementalSourceUrl":"https://platform.claude.com/","checkedAt":"2026-10-07"}}],"estimateMissingReasons":[{"provider":"codex","phase":"audit","requestedModel":"unknown","reason":"UNKNOWN_MODEL"}]},"byPhase":{"audit":{"calls":3,"costUsd":0.3,"estimatedCostUsd":0.15,"estimateMissing":1}}}}`)
	for i, lang := range []string{"en", "ko"} {
		md, err := RenderReport(report, lang)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"$0.4500", "$0.3000", "$0.1500", "30m02s", "2026-10-07", "| 50,000 | 50,000 | 10,000 | 1,000 | 1,350 | $0.1500 |", []string{"Unpriced", "미산정"}[i], []string{"No bundled rate", "단가표에 없는 모델"}[i]} {
			if !strings.Contains(md, want) {
				t.Fatalf("%s report lacks %s", lang, want)
			}
		}
		if strings.Index(md, "$0.4500") > strings.Index(md, []string{"## Committed changes", "## 커밋된 변경"}[i]) {
			t.Fatal("cost summary is not before committed changes")
		}
	}
	unknown, err := RenderReport([]byte(`{"schemaVersion":3,"status":"DONE","costPolicy":"provider_reported_then_standard_estimate","usage":{"totals":{"processes":1,"estimateMissing":1}}}`), "en")
	if err != nil || !strings.Contains(unknown, "Unavailable") || strings.Contains(unknown, "$0.0000") {
		t.Fatal("unknown cost became zero", err)
	}
}

func TestReportViewsPreserveFiles(t *testing.T) {
	repo := testRepo(t)
	run := filepath.Join(repo, ".refactor", "runs", "old")
	if err := os.MkdirAll(run, 0755); err != nil {
		t.Fatal(err)
	}
	report := []byte("{\n \"schemaVersion\": 3, \"status\": \"DONE\", \"codeComparison\": {\"status\":\"AVAILABLE\",\"preview\":\"+``````\\n\",\"patchFile\":\"changes.patch\",\"files\":[{\"path\":\"a|b\\n.txt\"}]}}\n")
	snapshots := map[string][]byte{"report.json": report, "report.md": []byte("saved markdown\n"), "changes.patch": []byte("+evidence\n"), "changes.md": []byte("- [x] reviewed\n")}
	for name, b := range snapshots {
		if err := os.WriteFile(filepath.Join(run, name), b, 0644); err != nil {
			t.Fatal(err)
		}
	}
	pointer, _ := json.Marshal(map[string]any{"schemaVersion": 1, "runDir": run})
	if err := os.WriteFile(filepath.Join(repo, ".refactor", "last-run.json"), pointer, 0644); err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"en", "ko"} {
		for _, raw := range []bool{false, true} {
			argv := []string{"report", "--lang", lang}
			if raw {
				argv = append(argv, "--json")
			}
			var out, stderr bytes.Buffer
			if code := Execute(argv, repo, &out, &stderr, Callbacks{}); code != 0 {
				t.Fatal(code, stderr.String())
			}
			if raw && !bytes.Equal(out.Bytes(), report) {
				t.Fatal("JSON bytes changed")
			}
			if !raw {
				for _, want := range []string{"```````diff\n+``````\n```````", "&#124;", "&#92;n", "(changes.patch)"} {
					if !strings.Contains(out.String(), want) {
						t.Fatalf("missing %s: %s", want, out.String())
					}
				}
			}
		}
	}
	for name, b := range snapshots {
		after, err := os.ReadFile(filepath.Join(run, name))
		if err != nil || !bytes.Equal(after, b) {
			t.Fatal("saved file changed", name, err)
		}
	}
}

func TestReportReasonsAndEmptyAudit(t *testing.T) {
	for reason, want := range map[string]string{
		"stopped on commit budget (1)": "커밋 한도 1개", "stopped on cycle budget (25)": "사이클 한도 25회", "stopped on wall clock (180m)": "경과 시간 한도 180분", "stopped on 3 consecutive failures": "후보가 3회 연속 실패", "stopped on all providers exhausted": "사용 가능한 프로바이더가 없습니다", "REGRESSION: src/a.ts:42 E123": "검증 회귀 (REGRESSION): src/a.ts:42 E123", "every proposed candidate was filtered by policy": "감사가 제안한 후보를 정책 필터가 모두 제외했습니다", "the audit proposed no candidates": "감사가 후보를 제안하지 않았습니다", "unknown error 그대로": "unknown error 그대로",
	} {
		b, _ := json.Marshal(map[string]any{"schemaVersion": 3, "reason": reason})
		ko, err := RenderReport(b, "ko")
		if err != nil || !strings.Contains(ko, want) {
			t.Fatalf("%s: %s %v", reason, ko, err)
		}
		en, _ := RenderReport(b, "en")
		if !strings.Contains(en, reason) {
			t.Fatal("raw reason lost")
		}
	}
	for _, lang := range []string{"en", "ko"} {
		out, err := RenderReport([]byte(`{"schemaVersion":3,"counters":{"auditsNoProposals":2,"auditsAllFiltered":3}}`), lang)
		if err != nil {
			t.Fatal(err)
		}
		want := "3 had every proposal filtered by policy"
		if lang == "ko" {
			want = "2회는 제안 없음"
		}
		if !strings.Contains(out, want) {
			t.Fatal(out)
		}
	}
	old, _ := RenderReport([]byte(`{"schemaVersion":3}`), "en")
	if strings.Contains(old, "Audits with no eligible candidate") {
		t.Fatal("invented audit counters")
	}
}

func TestConfigSampleMatchesDefaults(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "config.default.json"))
	if err != nil {
		t.Fatal(err)
	}
	var sample Config
	if err := json.Unmarshal(data, &sample); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sample, DefaultConfig()) {
		t.Fatal("configuration example differs from installed defaults")
	}
	for _, name := range []string{"codex", "claude"} {
		agent := obj(obj(sample["agents"])[name])
		if agent["effort"] != nil || len(obj(agent["effort_by_phase"])) != 0 {
			t.Fatal("sample overrides per-phase effort policy")
		}
	}
}

func TestMissingReportPreservesMarkdown(t *testing.T) {
	repo := testRepo(t)
	run := filepath.Join(repo, ".refactor", "runs", "old")
	if err := os.MkdirAll(run, 0755); err != nil {
		t.Fatal(err)
	}
	md := filepath.Join(run, "report.md")
	if err := os.WriteFile(md, []byte("existing evidence"), 0644); err != nil {
		t.Fatal(err)
	}
	pointer, _ := json.Marshal(map[string]any{"schemaVersion": 1, "runDir": run})
	if err := os.WriteFile(filepath.Join(repo, ".refactor", "last-run.json"), pointer, 0644); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if code := Execute([]string{"report"}, repo, &out, &stderr, Callbacks{}); code != ExitAborted || out.Len() != 0 {
		t.Fatal(code, out.String(), stderr.String())
	}
	b, err := os.ReadFile(md)
	if err != nil || string(b) != "existing evidence" {
		t.Fatal("Markdown overwritten")
	}
}
