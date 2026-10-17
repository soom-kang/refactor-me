package surface

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReportUsesSeparateChangesMarkdown(t *testing.T) {
	for _, language := range []string{"en", "ko"} {
		for _, status := range []string{"AVAILABLE", "NO_CHANGES"} {
			t.Run(language+"/"+status, func(t *testing.T) {
				comparison := map[string]any{
					"status": status, "markdownFile": "changes.md", "patchFile": "changes.patch",
					"preview": "+inline preview must stay separate\n", "truncated": true,
					"totals": map[string]any{"files": 1, "insertions": 1, "deletions": 0, "binary": 0},
					"files":  []any{map[string]any{"path": "app/a.go", "status": "M", "insertions": 1, "deletions": 0}},
				}
				data, err := json.Marshal(map[string]any{"schemaVersion": 3, "status": "DONE", "codeComparison": comparison})
				if err != nil {
					t.Fatal(err)
				}
				out, err := RenderReport(data, language)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(out, "(changes.md)") {
					t.Fatal("separate Markdown link missing", out)
				}
				for _, unwanted := range []string{"(changes.patch)", "```diff", "+inline preview", "200", "32 KiB"} {
					if strings.Contains(out, unwanted) {
						t.Fatalf("report still includes %q: %s", unwanted, out)
					}
				}
				if status == "AVAILABLE" {
					for _, want := range []string{"app/a.go", "| 1 | 0 |", label(language, "Rejected or rolled-back edits are excluded.", "거절되거나 롤백된 수정은 제외합니다.")} {
						if !strings.Contains(out, want) {
							t.Fatalf("comparison summary lacks %q: %s", want, out)
						}
					}
				} else if !strings.Contains(out, label(language, "No committed code changes.", "커밋된 코드 변경이 없습니다.")) {
					t.Fatal("no-change state missing", out)
				}
			})
		}
	}
}

func TestReportFallsBackWithoutChangesMarkdown(t *testing.T) {
	for _, language := range []string{"en", "ko"} {
		for _, markdownError := range []string{"", "cannot save Markdown"} {
			t.Run(language+"/"+markdownError, func(t *testing.T) {
				comparison := map[string]any{
					"status": "AVAILABLE", "patchFile": "changes.patch", "markdownError": markdownError,
					"preview": "+legacy preview\n", "truncated": true,
				}
				out := strings.Join(renderCodeComparison(comparison, language), "\n")
				for _, want := range []string{"(changes.patch)", "```diff\n+legacy preview\n```", "200", "32 KiB"} {
					if !strings.Contains(out, want) {
						t.Fatalf("fallback lacks %q: %s", want, out)
					}
				}
				if strings.Contains(out, "(changes.md)") {
					t.Fatal("fallback points to missing Markdown", out)
				}
				if markdownError != "" && !strings.Contains(out, markdownError) {
					t.Fatal("Markdown save failure missing", out)
				}
				comparison["status"] = "NO_CHANGES"
				out = strings.Join(renderCodeComparison(comparison, language), "\n")
				if !strings.Contains(out, "(changes.patch)") || !strings.Contains(out, label(language, "No committed code changes.", "커밋된 코드 변경이 없습니다.")) || strings.Contains(out, "```diff") {
					t.Fatal("no-change fallback changed", out)
				}
			})
		}
	}
}
