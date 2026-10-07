package surface

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
)

const maxRuntimeMinutes = math.MaxInt64 / int64(time.Minute)

func validMaxMinutes(minutes int) bool {
	return minutes > 0 && int64(minutes) <= maxRuntimeMinutes
}

func parseMaxMinutes(value string) (int, error) {
	minutes, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || minutes < 1 || minutes > maxRuntimeMinutes {
		return 0, fmt.Errorf("use a whole number from 1 to %d", maxRuntimeMinutes)
	}
	return int(minutes), nil
}

func configuredMaxMinutes(cfg Config) (int, error) {
	minutes := 180
	switch value := object(cfg["policy"])["max_wall_clock_min"].(type) {
	case int:
		minutes = value
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 1 || value > float64(maxRuntimeMinutes) {
			return 0, errors.New("policy.max_wall_clock_min must be positive and fit a time duration")
		}
		// Match the controller's existing numeric configuration conversion.
		minutes = int(value)
	}
	if !validMaxMinutes(minutes) {
		return 0, errors.New("policy.max_wall_clock_min must be positive and fit a time duration")
	}
	return minutes, nil
}

// runDurationOptions changes only the effective run config, never the project file.
func runDurationOptions(cfg Config, args Args, selectMinutes func(int, string) (int, error)) (Config, string, error) {
	if args.Command != "run" {
		return cfg, "", nil
	}
	minutes, source := args.MaxMinutes, "cli"
	if minutes == 0 {
		var err error
		minutes, err = configuredMaxMinutes(cfg)
		if err != nil {
			return nil, "", err
		}
		source = "config"
		if !args.JSON && selectMinutes != nil {
			minutes, err = selectMinutes(minutes, args.Language)
			if err != nil {
				return nil, "", err
			}
			source = "interactive"
		}
	}
	if !validMaxMinutes(minutes) {
		return nil, "", errors.New("run time limit must be positive and fit a time duration")
	}
	resolved := Config(mergeObject(map[string]any(cfg), nil))
	policy := mergeObject(object(cfg["policy"]), nil)
	policy["max_wall_clock_min"] = minutes
	resolved["policy"] = policy
	return resolved, source, nil
}

// SelectMaxMinutes asks once per interactive run, before any provider or worktree work.
func SelectMaxMinutes(stdin io.Reader, stderr io.Writer, defaultMinutes int, language string) (int, error) {
	if !validMaxMinutes(defaultMinutes) {
		return 0, errors.New("invalid configured run time limit")
	}
	reader := bufio.NewReader(stdin)
	write := func(en, ko string, args ...any) error {
		_, err := fmt.Fprintf(stderr, label(language, en, ko), args...)
		return err
	}
	read := func() (string, error) {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", errors.New(label(language, "run cancelled before starting: could not read a time selection", "시간을 선택하지 못해 실행 시작 전에 취소했습니다"))
		}
		value := strings.ToLower(strings.TrimSpace(line))
		if value == "q" || value == "cancel" || value == "\x03" || value == "\x1b" {
			return "", errors.New(label(language, "run cancelled before starting", "실행 시작 전에 취소했습니다"))
		}
		return value, nil
	}
	if err := write("Maximum run time: 30 / 60 / 180 / 360 minutes / custom\nThe current work unit finishes before stopping.\n", "최대 실행 시간: 30 / 60 / 180 / 360분 / custom\n시간 한도에 도달하면 진행 중인 작업 단위를 마친 뒤 종료합니다.\n"); err != nil {
		return 0, err
	}
	for {
		if err := write("Choose a listed limit or custom; Enter keeps %d minutes, q cancels: ", "선택값 또는 custom을 입력하세요. Enter: 기존 %d분 유지, q: 취소: ", defaultMinutes); err != nil {
			return 0, err
		}
		value, err := read()
		if err != nil {
			return 0, err
		}
		if value == "" {
			return defaultMinutes, nil
		}
		if value == "custom" || value == "c" {
			for {
				if err := write("Custom limit in whole minutes, q cancels: ", "최대 실행 시간을 분 단위 정수로 입력하세요. q: 취소: "); err != nil {
					return 0, err
				}
				value, err = read()
				if err != nil {
					return 0, err
				}
				if minutes, err := parseMaxMinutes(value); err == nil {
					return minutes, nil
				}
				if err := write("Enter a positive whole number of minutes.\n", "1분 이상의 정수를 입력하세요.\n"); err != nil {
					return 0, err
				}
			}
		}
		switch value {
		case "30", "60", "180", "360":
			minutes, _ := parseMaxMinutes(value)
			return minutes, nil
		}
		if err := write("Choose 30, 60, 180, 360, or custom.\n", "30, 60, 180, 360 또는 custom을 입력하세요.\n"); err != nil {
			return 0, err
		}
	}
}
