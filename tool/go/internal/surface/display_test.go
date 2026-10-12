package surface

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestDisplayTextPreservesUnicodeAndRemovesTerminalControl(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"한국어\n경로\tname\r\x1b[31m red\x1b[0m", "한국어 경로 name red"},
		{"a\x1b]0;malicious title\ab", "ab"},
		{"a\x1b]8;;https://example.invalid\x1b\\link\x1b]8;;\x1b\\b", "alinkb"},
		{"a\x1b[", "a"},
		{"a\x1b]unterminated", "a"},
		{"a\u202eb\x00", "ab"},
	} {
		if got := DisplayText(tc.input); got != tc.want {
			t.Fatalf("%q => %q, want %q", tc.input, got, tc.want)
		}
	}
	got := DisplayText(strings.Repeat("한", 200))
	if !utf8.ValidString(got) || utf8.RuneCountInString(got) != 160 || !strings.HasSuffix(got, "…") {
		t.Fatal(got)
	}
}

func TestTerminalTextPreservesReportLayout(t *testing.T) {
	value := "## 한국어\n```diff\n+new\tvalue\r\n-old\n```\n"
	want := strings.ReplaceAll(value, "\r", "")
	if got := terminalText(value, true); got != want {
		t.Fatalf("report layout changed: %q", got)
	}
	for _, control := range []string{
		"\x1b[2J", "\x1b]0;title\a", "\x1b]52;c;payload\x1b\\", "\u009b2J", "\u009d0;title\u009c", "\x1bPpayload\x1b\\", "\x00\a\b\r\u202e",
	} {
		if got := terminalText("before"+control+"after", true); got != "beforeafter" {
			t.Fatalf("control retained: %q", got)
		}
	}
	if got := terminalText(strings.Repeat("한", 200), true); utf8.RuneCountInString(got) != 200 {
		t.Fatal("full report text was limited as a label")
	}
}

func TestReportTruncationPreservesByteBudgetAndUTF8(t *testing.T) {
	for _, tc := range []struct {
		input string
		limit int
		want  string
	}{
		{strings.Repeat("가", 67), 200, strings.Repeat("가", 66)},
		{strings.Repeat("a", 117) + "🙂end", 120, strings.Repeat("a", 117)},
		{"가🙂a", 7, "가🙂"},
		{"가", 1, ""},
		{"abc", 2, "ab"},
		{"abc", 0, ""},
	} {
		if got := truncate(tc.input, tc.limit); got != tc.want || !utf8.ValidString(got) || len(got) > tc.limit {
			t.Fatalf("truncate(%q, %d) = %q", tc.input, tc.limit, got)
		}
	}
}

func TestStopReasonDoesNotExposeOriginalUnknownError(t *testing.T) {
	for _, lang := range []string{"en", "ko"} {
		for _, reason := range []string{"internal error: private notes", "codex: raw provider prose", "REGRESSION: private output"} {
			got := StopReason(reason, lang)
			if strings.Contains(got, "private") || strings.Contains(got, "provider prose") || got == "" {
				t.Fatal(got)
			}
		}
	}
	if got := StopReason("stopped on cycle budget (3)", "ko"); !strings.Contains(got, "3") || !strings.Contains(got, "한도") {
		t.Fatal(got)
	}
}
