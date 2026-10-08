![refactor-me](assets/refactor-me-title.png)

# refactor-me

[![Release](https://img.shields.io/badge/Release-0.10.0--beta.6-2f6f5e)](https://github.com/soom-kang/refactor-me/releases/tag/v0.10.0-beta.6) [![Verify](https://img.shields.io/github/actions/workflow/status/soom-kang/refactor-me/verify.yml?branch=main&label=Verify)](https://github.com/soom-kang/refactor-me/actions/workflows/verify.yml) [![MIT License](https://img.shields.io/badge/License-MIT-555555)](../LICENSE)

refactor-me는 Codex 또는 Claude Code와 sharpen-me 스킬을 활용해, 기존 동작을 보존하는 리팩토링을 보수적인 절차로 진행하는 CLI입니다.

“리팩토링해 줘”라는 요청만으로는 변경의 위험을 어떻게 평가하고, 어떤 검증과 리뷰를 거칠지 충분히 정하기 어렵습니다. refactor-me는 이 과정을 명시적으로 관리하기 위해 만들었습니다. 후보 평가와 영향 분석, 필요한 테스트 보강, 사전 점검(preflight), 수정, 검증, 별도 세션의 리뷰를 단계별로 진행하고 판단 근거와 결과를 기록합니다.

변경은 격리된 Git worktree에서 수행하며, 검증과 리뷰를 통과한 커밋을 로컬 브랜치에 남깁니다. 사용자는 보고서와 변경 내용을 확인한 뒤 반영 여부를 결정합니다.

[English](../README.md) · [사용법](../tool/TUTORIAL.ko.md) · [명령·설정](../tool/README.ko.md) · [동작 방식](../tool/WORKFLOW.ko.md)

## 설치

공개 Beta **0.10.0-beta.6**은 **macOS Apple Silicon**을 지원합니다. Homebrew, Git, 인증을 마친 Codex 또는 Claude Code CLI와 대상 프로젝트의 빌드·테스트 도구가 필요합니다.

릴리스 commit은 `01b407ce81e9803878c003c181860e499718b0fa`입니다. 이 버전의 [릴리스 검증](https://github.com/soom-kang/refactor-me/actions/runs/37625020035), [새 설치 검증](https://github.com/soom-kang/refactor-me/actions/runs/37625488585), [Beta.5에서의 upgrade 검증](https://github.com/soom-kang/refactor-me/actions/runs/37625494290)이 통과했습니다. 설치 검증은 모델을 호출하지 않았으며 upgrade에는 저장된 프로젝트 상태를 확인하는 seeded 호환성 보고서를 사용했습니다. Verify 배지는 CLI 저장소 `main`의 workflow 상태를 표시합니다.

<a id="quick-start"></a>

```sh
brew tap soom-kang/refactor-me
brew trust --formula soom-kang/refactor-me/refactor-me
brew install refactor-me
refactor-me version --json
```

Homebrew 6 이상에서 tap 등록과 해당 formula의 trust 설정은 처음 한 번만 합니다. 이후에는 `brew upgrade refactor-me`로 갱신합니다. 새 Homebrew 환경에서는 위 준비 명령을 먼저 실행해야 짧은 설치 명령을 사용할 수 있습니다.

고정된 sharpen-me [Skill 기준 카탈로그](../tool/release/INSTALL.ko.md#skill-reference)를 전역 설치하세요. Git을 쓰는 이 경로에는 Node.js나 버전이 고정되지 않은 설치기가 필요하지 않습니다.

Homebrew가 실행 파일을 관리하고, CLI는 `~/.agents/skills`에서 필수 Skills를 읽습니다. 각 대상 저장소에는 설정과 실행 기록을 따로 저장합니다.

![전역 CLI와 Skill을 여러 Git 저장소에서 사용하며, 각 저장소는 .refactor 상태를 따로 보관합니다.](assets/workflow/installation.ko.png)

## 첫 실행

아래 명령을 실행하기 전에 `refactor-me version --json`이 `0.10.0-beta.6`을 표시하는지 확인하세요. 이전 버전이라면 먼저 `brew upgrade refactor-me`를 실행합니다.

아래 경로를 커밋이 하나 이상 있는 깨끗한 Git 저장소로 바꾸세요. 프로젝트 의존성도 먼저 준비합니다. 예시는 Codex만 사용하며 Claude Code 검사에는 `--provider claude --fallback none`을 사용합니다.

```sh
refactor-me init --repo /path/to/target-repo
refactor-me doctor --repo /path/to/target-repo \
  --provider codex --fallback none --no-live-probe
```

`init`은 기존 설정을 덮어쓰지 않습니다. 첫 실행 제한을 파일로 설정하려면 먼저 실행하세요. 그 외에는 초기화를 생략할 수 있습니다. 위 doctor 명령은 모델을 호출하지 않습니다. **실행 전에 [첫 실행 제한](../tool/TUTORIAL.ko.md#set-limits-and-run)을 설정하세요.**

```sh
refactor-me run --repo /path/to/target-repo \
  --provider codex --fallback none \
  --model gpt-6.1-sol --effort xhigh
refactor-me report --repo /path/to/target-repo --lang ko
```

선택한 provider마다 명령 옵션이나 프로젝트 설정으로 모델을 지정하세요. 예시의 모델과 추론 수준은 이번 실행의 선택값이며 기본값이 아닙니다. Claude Code는 `--provider claude --fallback none --model claude-sonnet-5-5 --effort xhigh`를 사용합니다. 이 옵션은 설정 파일을 바꾸지 않습니다. fallback 예시와 우선순위는 [모델과 추론 수준 선택](../tool/README.ko.md#model-and-effort-selection)을 참고하세요.

`run`과 `--no-live-probe` 없는 doctor는 provider 사용량을 소비합니다. 종료 코드가 `0`이어도 부분 완료일 수 있으므로 보고서와 diff를 확인한 뒤 병합하세요. CLI는 결과를 자동으로 병합하거나 push·배포하지 않습니다.

## 읽기 쉬운 진행 로그

공개 Beta `0.10.0-beta.6`은 shell 명령 대신 현재 단계, 작업 항목과 확인된 결과를 표시합니다. 기존 `--lang en|ko`로 진행 로그, 종료 요약과 보고서의 언어를 선택하며 기본값은 영어입니다. 한국어 진행 로그에는 `--lang ko`를 추가하세요.

진행 로그는 `stderr`로 출력하므로 `run --json`의 `stdout`에는 보고서 JSON만 남습니다. 조사할 때마다 제안된 후보 수와 진행 가능한 수를 표시하고, 긴 단계에서는 30초마다 경과 시간을 알립니다. provider 응답 원문과 검증 출력은 로컬 실행 기록에 보존합니다. 자세한 내용은 [진행 로그](../tool/README.ko.md#progress-logs)를 참고하세요.

## 실행 제한과 보고서

실행과 결과 검토에 아래 기능을 사용합니다.

- `--max-minutes 60`으로 이번 실행의 시간을 정합니다. macOS 터미널에서는 30, 60, 180, 360분이나 custom을 선택하며 Enter는 프로젝트의 기존 한도를 유지합니다. JSON 출력이나 입출력 리다이렉션에서는 질문하지 않습니다. 진행 중인 작업 단위를 마치는 동안 한도를 초과할 수 있습니다.
- 기본 진행 로그에 경과 시간, 단계, provider와 안전한 파일 및 도구 작업을 표시합니다. 명령 원문, 출력과 민감 경로는 작업 로그에 표시하지 않습니다.
- `changes.md`에서 파일 검토 체크리스트와 커밋된 전체 텍스트 diff를 확인합니다. 비용은 provider 보고값을 우선하고, 지원하는 모델의 미보고 비용에는 확인 날짜를 기록한 [Artificial Analysis](https://artificialanalysis.ai/) 표준 API 추정값을 사용합니다. 구독 청구액과 구분하며 알 수 없는 사용량은 그대로 표시합니다.

[시간 선택](../tool/README.ko.md#run-time-selection), [코드 비교](../tool/README.ko.md#code-comparison), [사용량](../tool/README.ko.md#usage-and-exit-codes)에서 세부 동작을 확인하세요.

## 안전 검사와 중단 처리

Beta.6은 characterization 테스트 변경을 커밋 전에 검사하고, 선언된 파일 삭제를 worktree 안으로 제한합니다. Ctrl-C 또는 SIGTERM을 받으면 관리 중인 provider와 검증 프로세스를 취소하고, 수락한 커밋과 미완료 작업은 보존합니다. 컨트롤러 준비 실패가 기록되면 `report`는 이전 성공 대신 해당 실패를 안내합니다. [중단과 실패한 실행](../tool/README.ko.md#interruption-and-failed-attempts), [실행 과정의 검사](../tool/WORKFLOW.ko.md#from-checks-to-a-local-branch)를 참고하세요.

## 상세 문서

| 할 일 | 문서 |
| --- | --- |
| 설치부터 대상 선택, 실행, 결과 확인까지 | [사용법](../tool/TUTORIAL.ko.md) |
| 명령, 설정, 종료 코드 확인 | [참조 문서](../tool/README.ko.md) |
| 격리, 검증, 검토 과정 이해 | [동작 방식](../tool/WORKFLOW.ko.md) |
| 개발 검증 또는 릴리스 준비 | [개발 안내](../tool/DEVELOPMENT.md) · [Homebrew 배포](../tool/release/HOMEBREW.ko.md) |
| 버전별 변경 확인 | [변경 기록](../tool/CHANGELOG.md) |

이 Beta는 Apple 서명·공증을 제공하지 않습니다. 직접 다운로드와 macOS 경고 대응은 [설치 안내](../tool/release/INSTALL.ko.md)를 참고하세요.

## 라이선스

[MIT](../LICENSE), copyright 2026 soom-kang. CLI나 문서를 재배포할 때 저작권과 라이선스 고지를 포함해야 합니다. 소프트웨어는 보증 없이 제공됩니다. sharpen-me는 자체 라이선스를, Codex와 Claude Code는 각 provider의 약관을 따릅니다.
