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
