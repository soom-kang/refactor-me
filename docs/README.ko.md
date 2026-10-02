![refactor-me](assets/refactor-me-title.png)

# refactor-me

Codex 또는 Claude Code로 기존 동작을 보존하는 리팩토링을 자동화합니다. 격리된 Git worktree에서 후보를 수정하고 검증한 뒤, 통과한 커밋을 검토용 로컬 branch에 저장합니다.

[English](../README.md) · [사용법](../tool/TUTORIAL.ko.md) · [명령·설정](../tool/README.ko.md) · [동작 방식](../tool/WORKFLOW.ko.md)

## 설치

공개 Beta **0.10.0-beta.1**은 **macOS Apple Silicon**을 지원합니다. Homebrew, Git, 인증을 마친 Codex 또는 Claude Code CLI와 대상 프로젝트의 빌드·테스트 도구가 필요합니다.

**릴리스 후보: `0.10.0-beta.2`.** 아래 모델과 추론 수준 옵션은 후보 버전에 적용됩니다. 아직 게시하지 않았으며 Homebrew는 현재 `0.10.0-beta.1`을 설치합니다.

<a id="quick-start"></a>

```sh
brew tap soom-kang/refactor-me
brew trust --formula soom-kang/refactor-me/refactor-me
brew install refactor-me
refactor-me version --json
```

Homebrew 6 이상에서 tap 등록과 해당 formula의 trust 설정은 처음 한 번만 합니다. 이후에는 `brew upgrade refactor-me`로 갱신합니다. 새 Homebrew 환경에서는 위 준비 명령을 먼저 실행해야 짧은 설치 명령을 사용할 수 있습니다.

sharpen-me Skills는 전역으로 설치합니다. 아래 설치기에는 Node.js가 필요하지만 refactor-me 실행에는 필요하지 않습니다.

```sh
npx skills add soom-kang/sharpen-me \
  --global --skill '*' --agent codex claude-code
```

Homebrew가 실행 파일을 관리하고, CLI는 `~/.agents/skills`에서 필수 Skills를 읽습니다. 각 대상 저장소에는 설정과 실행 기록을 따로 저장합니다.

![전역 CLI와 Skill을 여러 Git 저장소에서 사용하며, 각 저장소는 .refactor 상태를 따로 보관합니다.](assets/workflow/installation.ko.png)

## 첫 실행

`refactor-me version --json`이 `0.10.0-beta.1`을 표시하면 아래 후보 명령을 실행하지 말고 [공개 `0.10.0-beta.1` 사용법](https://github.com/soom-kang/refactor-me/blob/v0.10.0-beta.1/tool/TUTORIAL.ko.md)을 따르세요. 검증한 `0.10.0-beta.2` 후보 실행 파일을 사용할 때만 아래 단계로 넘어갑니다.

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
