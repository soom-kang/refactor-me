![refactor-me](assets/refactor-me-title.png)

# refactor-me

[![Verify](https://github.com/soom-kang/refactor-me/actions/workflows/verify.yml/badge.svg)](https://github.com/soom-kang/refactor-me/actions/workflows/verify.yml) [![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](../LICENSE)

Codex와 Claude Code로 동작을 보존하는 리팩터링을 실행합니다. CLI는 별도 worktree에서 수정·검증하고 결과를 로컬 `refactor/auto-*` 브랜치에 저장합니다. 병합 전에 diff와 리포트를 검토하세요.

**공개 Beta:** macOS Apple Silicon용 `0.10.0-beta.1`을 전용 Homebrew tap으로 배포합니다. 개발 빌드는 `dev`로 표시합니다.

[English](../README.md) · [실행 가이드](../tool/TUTORIAL.ko.md) · [참조](../tool/README.ko.md) · [Workflow](../tool/WORKFLOW.ko.md) · [변경 기록](../tool/CHANGELOG.md)

## 설치

macOS Apple Silicon, Homebrew, Git, 인증된 `codex` 또는 `claude` CLI와 대상 프로젝트의 검증 도구를 준비하세요. Homebrew formula는 고정된 소스를 빌드하며 Go를 빌드 의존성으로 설치합니다. refactor-me 실행에는 Go나 Node 런타임이 필요하지 않습니다. 아래 외부 Skill 설치 명령에는 Node.js가 필요합니다.

Tap과 릴리스가 공개된 뒤 실행하세요.

```sh
brew install soom-kang/refactor-me/refactor-me
refactor-me version --json
npx skills add \
  https://github.com/soom-kang/sharpen-me/tree/v0.9.0-beta.2 \
  --global --skill '*' --agent codex claude-code
```

필수 Skill 8개의 공통 원본은 `~/.agents/skills`입니다. 대상 프로젝트에 Skill을 커밋하거나 실행 파일을 복사할 필요가 없습니다. `doctor`는 누락 파일과 이름 충돌을 경로와 함께 알리며 자동 삭제하지 않습니다. 같은 원본으로 해석되는 provider 링크는 허용합니다.

개발 중에는 Go 1.27로 소스를 빌드할 수 있습니다.

```sh
git clone https://github.com/soom-kang/refactor-me.git
cd refactor-me/tool/go
go build -o /private/tmp/refactor-me ./cmd/refactor-me
/private/tmp/refactor-me version --json
/private/tmp/refactor-me doctor --repo /path/to/target-repo --no-live-probe
```

개발 빌드를 사용할 때는 아래 명령의 `refactor-me`를 `/private/tmp/refactor-me`로 바꿔 실행하세요.

이 Beta는 Apple 서명·공증을 제공하지 않습니다. 다운로드한 바이너리의 checksum은 바이트 변경을 확인하며 게시자 신원을 인증하지는 않습니다. macOS가 차단하면 [Apple의 개별 앱 열기 안내](https://support.apple.com/en-gb/102445)를 따르세요. 시스템 전체 보안 설정을 끄지 마세요.

## 실행

대상 checkout을 깨끗하게 준비하고 첫 실행 전에 [실행 한도](../tool/TUTORIAL.ko.md#set-limits-and-run)를 설정하세요.

```sh
refactor-me init --repo /path/to/target-repo
refactor-me doctor --repo /path/to/target-repo --no-live-probe
refactor-me run --repo /path/to/target-repo --provider codex --fallback none
```

Claude만 사용한다면 `doctor`와 `run`에 `--provider claude --fallback none`을 지정하세요. 두 옵션을 생략하면 Codex를 우선하고 Claude를 대체 provider로 사용합니다.

`init`은 선택 사항이며 기존 설정을 덮어쓰지 않습니다. 설정 파일이 없어도 `run`은 내장 기본값으로 실행됩니다. 인자 없는 `refactor-me`는 도움말을 표시합니다. `doctor`에서 `--no-live-probe`를 생략하면 모델을 호출해 계정 사용량을 소비합니다. 디스크 검사만으로 실제 세션의 Skill 로딩을 입증할 수는 없습니다.

대상 저장소 안에서는 `--repo`를 생략할 수 있습니다.

```sh
refactor-me run --target app/web
refactor-me run --target app/web --target app/api --fallback claude
```

`--repo`를 지정한 경우 상대 target은 선택한 Git 루트를 기준으로 합니다. 생략하면 현재 디렉터리가 기준입니다. `--target`은 후보 탐색 범위이며 호출부 확인·수정과 검증은 저장소 안의 다른 영역까지 이어질 수 있습니다. [범위와 검증 규칙](../tool/README.ko.md#후보-선택)을 확인하세요.

CLI는 로컬 커밋과 결과 브랜치를 만듭니다. merge·push·배포는 수행하지 않습니다. 실행 중 원본 checkout을 변경하지 마세요.

## 리포트

```sh
refactor-me report --repo /path/to/target-repo
refactor-me report --repo /path/to/target-repo --lang ko
refactor-me report --repo /path/to/target-repo --json
```

리포트에는 파일 통계, diff 미리보기와 `changes.patch`가 있습니다. 조회는 모델을 호출하거나 파일을 덮어쓰지 않습니다. 새 실행의 한국어 리포트는 `run --lang ko`로 선택합니다. 현재 설정은 schema 2, 리포트는 schema 3입니다. 지원하지 않는 과거 형식에는 오류를 내며 자동 변환·삭제하지 않습니다. [리포트 형식과 제한사항](../tool/README.ko.md#리포트와-언어)을 참고하세요.

## 업데이트와 제거

```sh
brew upgrade soom-kang/refactor-me/refactor-me
brew uninstall refactor-me
```

업데이트는 모든 프로젝트에서 사용하는 CLI 버전을 바꿉니다. 제거해도 프로젝트의 설정·실행 기록·결과 브랜치·worktree와 전역 Skills는 남습니다. 프로젝트별 `install`·`uninstall` 명령은 더 이상 제공하지 않습니다. 기존 로컬 바이너리는 자동 삭제하지 않으므로 `command -v refactor-me`와 `version --json`으로 실행 경로를 확인하세요.

## 검증

이 저장소 루트에서 실행하세요.

```sh
cd tool/go
go test -race -count=1 ./...
go vet ./...
go build -o /private/tmp/refactor-me ./cmd/refactor-me
/private/tmp/refactor-me version --json
```

[Fixture 가이드](../tool/fixtures/README.ko.md)는 Go 예제와 선택적 JavaScript 검증을 설명합니다. 기본 개발 검사에는 Node가 필요하지 않습니다. 실제 provider의 Skill 로딩과 모델 판단은 별도 실행으로 확인해야 합니다. [검증 범위](../tool/README.ko.md#로컬-개발-검사)를 참고하세요.


유지보수자는 [Homebrew 릴리스 가이드](../tool/release/HOMEBREW.md)에서 소스 압축파일·formula 생성과 게시 조건을 확인할 수 있습니다.

## 라이선스

CLI와 문서는 [MIT License](../LICENSE)를 적용하며 저작권자는 2026 soom-kang입니다. 필수 sharpen-me Skills는 각자의 MIT 라이선스 파일을 유지합니다. Codex와 Claude Code는 각 공급자의 약관을 따르는 외부 선행 도구입니다.
