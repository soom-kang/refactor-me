package surface

import (
	"strings"
	"unicode"
)

// DisplayText bounds untrusted terminal labels without changing stored evidence.
func DisplayText(value string) string {
	var out strings.Builder
	runes := []rune(value)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\x1b' {
			i++
			if i >= len(runes) {
				break
			}
			switch runes[i] {
			case '[': // CSI, including an incomplete sequence.
				for i++; i < len(runes); i++ {
					if runes[i] >= '@' && runes[i] <= '~' {
						break
					}
				}
			case ']': // OSC, terminated by BEL or ST.
				for i++; i < len(runes); i++ {
					if runes[i] == '\a' {
						break
					}
					if runes[i] == '\x1b' && i+1 < len(runes) && runes[i+1] == '\\' {
						i++
						break
					}
				}
			}
			continue
		}
		if unicode.IsSpace(r) {
			out.WriteByte(' ')
		} else if !unicode.IsControl(r) && !unicode.Is(unicode.Cf, r) {
			out.WriteRune(r)
		}
	}
	clean := strings.Join(strings.Fields(out.String()), " ")
	runes = []rune(clean)
	if len(runes) > 160 {
		return string(runes[:159]) + "…"
	}
	return clean
}

// ReasonLabel describes a known outcome code without rendering model notes.
func ReasonLabel(code, language string) string {
	extra := map[string][2]string{
		"VALIDATION_UNRUNNABLE":     {"Validation command could not start", "검증 명령 실행 불가"},
		"VALIDATION_FAIL":           {"Validation failed", "검증 실패"},
		"PREFLIGHT_FAILED":          {"Preflight could not complete", "구현 준비 확인 실패"},
		"EMPTY_ALLOWLIST":           {"No allowed files were specified", "수정할 파일이 지정되지 않음"},
		"NO_CHARACTERIZATION_FILES": {"No characterization test files were specified", "동작 확인 테스트 파일이 지정되지 않음"},
		"CHARACTERIZATION_FAILED":   {"Characterization could not complete", "동작 확인 테스트 작성 실패"},
	}
	if pair, ok := extra[code]; ok {
		return label(language, pair[0], pair[1])
	}
	if language == "ko" {
		if text, ok := reasonKO[code]; ok {
			return text
		}
		return "작업을 진행하지 못했습니다"
	}
	if text, ok := reasonEN[code]; ok {
		return text
	}
	return "The work could not continue"
}

// StopReason translates controlled terminal reasons, not provider/error prose.
func StopReason(reason, language string) string {
	switch reason {
	case "no eligible candidates remain", "every proposed candidate was filtered by policy", "the audit proposed no candidates", "doctor found a blocking problem", "no deterministic validation command could be discovered", "stopped on all providers exhausted":
		return translateReason(reason, language)
	case "no usable GREEN validation command at baseline":
		return label(language, "No validation command passed before changes", "변경 전 검증을 통과한 명령이 없습니다")
	case "stopped on all providers exhausted during audit":
		return label(language, "No provider remained available during the audit", "후보 조사 중 사용 가능한 provider가 없어졌습니다")
	case "audit failed without a usable provider response":
		return label(language, "The audit did not produce a usable response", "후보 조사에서 사용할 수 있는 응답을 얻지 못했습니다")
	}
	// Budget reasons are generated locally with numeric values.
	for _, prefix := range []string{"stopped on cycle budget (", "stopped on commit budget (", "stopped on wall clock (", "stopped on "} {
		if strings.HasPrefix(reason, prefix) {
			value := strings.TrimPrefix(reason, prefix)
			value = strings.TrimSuffix(value, " consecutive failures")
			value = strings.TrimSuffix(value, "m)")
			value = strings.TrimSuffix(value, ")")
			if value != "" && strings.Trim(value, "0123456789") == "" {
				return translateReason(reason, language)
			}
		}
	}
	if code, _, ok := strings.Cut(reason, ": "); ok {
		if _, known := reasonEN[code]; known {
			return ReasonLabel(code, language) + " [" + code + "]"
		}
	}
	return label(language, "A safety check or internal operation prevented continuing", "안전 검사 또는 내부 처리 문제로 작업을 중단했습니다")
}
