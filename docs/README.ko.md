![refactor-me](assets/refactor-me-title.png)

# refactor-me

[![Verify](https://github.com/soom-kang/refactor-me/actions/workflows/verify.yml/badge.svg)](https://github.com/soom-kang/refactor-me/actions/workflows/verify.yml) [![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](../LICENSE)

Codex와 Claude Code로 동작을 유지하는 리팩터링을 실행하는 CLI입니다. 별도 worktree에서 수정과 검증을 진행하고 결과를 `refactor/auto-*` 로컬 브랜치에 남깁니다. 병합 전에는 diff와 리포트를 확인하세요.

**Go 개발 빌드:** `dev`. 첫 Go Beta 릴리스는 macOS Apple Silicon용 `0.9.20-beta.1`입니다.

[English](../README.md) · [실행 가이드](../tool/TUTORIAL.ko.md) · [상세 문서](../tool/README.ko.md) · [Workflow](../tool/WORKFLOW.ko.md) · [변경 이력](../tool/CHANGELOG.md)

## 설치

먼저 다음 조건을 준비하세요.

- macOS Apple Silicon과 Git. Go 1.27은 소스에서 빌드할 때만 필요합니다.
- 인증된 `codex` 또는 `claude` CLI와 네트워크 연결
- 별도로 릴리스된 필수 의존성 [sharpen-me](https://github.com/soom-kang/sharpen-me)의 Skill 8개
- 의존성을 설치하고 빌드 캐시를 준비한 대상 프로젝트

1. [`v0.9.20-beta.1` 릴리스](https://github.com/soom-kang/refactor-me/releases/tag/v0.9.20-beta.1)에서 macOS Apple Silicon용 압축파일과 checksum 파일을 받으세요. 압축을 풀거나 실행하기 전에 검증하고, 예시 저장소 경로를 바꾸세요.

```bash
RELEASE_DIR="$(mktemp -d)"
(
set -e
RELEASE_VERSION=0.9.20-beta.1
RELEASE_URL="https://github.com/soom-kang/refactor-me/releases/download/v${RELEASE_VERSION}"
curl -fL "$RELEASE_URL/refactor-me_${RELEASE_VERSION}_darwin_arm64.zip" -o "$RELEASE_DIR/refactor-me_${RELEASE_VERSION}_darwin_arm64.zip"
curl -fL "$RELEASE_URL/SHA256SUMS" -o "$RELEASE_DIR/SHA256SUMS"
cd "$RELEASE_DIR"
shasum -a 256 -c SHA256SUMS
unzip -q "refactor-me_${RELEASE_VERSION}_darwin_arm64.zip"
cat INSTALL.md
RELEASE_INFO="$(./refactor-me version --json)"
printf '%s\n' "$RELEASE_INFO"
printf '%s\n' "$RELEASE_INFO" | grep -Fq "\"version\": \"$RELEASE_VERSION\""
printf '%s\n' "$RELEASE_INFO" | grep -Fq '"platform": "darwin"'
printf '%s\n' "$RELEASE_INFO" | grep -Fq '"arch": "arm64"'
./refactor-me install /path/to/target-repo
)
```

압축파일에는 `refactor-me`, `LICENSE`, `INSTALL.md`, `BUILD-INFO.txt`가 들어갑니다. 설치 전에 `version --json`이 릴리스 버전과 같은지 확인하고 `INSTALL.md`를 읽으세요. `SHA256SUMS`는 내려받은 파일의 변경을 확인하지만 게시자 신원을 단독으로 증명하지는 않습니다. 첫 Beta는 **서명하지 않고 공증하지 않습니다**. macOS가 실행을 차단하거나 경고할 수 있습니다. 이 경우 [Apple의 개별 앱 열기 안내](https://support.apple.com/en-gb/102445)를 따르세요. 시스템 전체의 보안 설정을 끄지 마세요.

개발용 빌드는 Go 1.27로 소스에서 만드세요.

```bash
git clone https://github.com/soom-kang/refactor-me.git
cd refactor-me/tool/go
go build -o /private/tmp/refactor-me ./cmd/refactor-me
/private/tmp/refactor-me version --json  # version은 dev
/private/tmp/refactor-me install /path/to/target-repo
```

2. **대상 저장소**에서 Skill을 설치하고 Project 범위를 선택하세요.

```bash
cd /path/to/target-repo
npx skills add soom-kang/sharpen-me --skill '*' --agent codex claude-code
```

3. 설치한 Skill 파일, 에이전트 링크와 `skills-lock.json`을 검토하고 커밋하세요. 실행용 worktree는 기준 커밋을 읽으므로 미커밋 Skill을 사용할 수 없습니다. Claude 전역 설치만으로는 부족합니다.

```bash
git add .agents .claude skills-lock.json && git commit
./.refactor/bin/refactor-me doctor --no-live-probe
```

설치기는 실체 파일을 `.agents/skills/<name>/`에 두고 `.claude/skills/<name>`은 그곳을 가리키는 심볼릭 링크로 만듭니다. 두 디렉터리를 함께 커밋하세요. 한쪽만 커밋하면 실행용 worktree에서 링크가 아무것도 가리키지 못합니다.

설치기는 Go 바이너리를 `.refactor/bin/refactor-me`에 복사하고 기존 설정과 실행 기록을 보존합니다. 식별할 수 있는 이전 설치물만 교체합니다. 소유 확인 오류가 나오면 덮어쓰기 전에 해당 파일을 확인하세요. 릴리스 바이너리를 실행하는 대상 저장소에는 Go가 필요하지 않습니다. `npx skills add`는 외부 Skill 설치 도구이므로 Node.js가 필요합니다. refactor-me 자체의 기본 개발·검증에는 Node가 필요하지 않으며, JavaScript/TypeScript 대상 프로젝트를 검증할 때는 해당 프로젝트의 도구가 필요합니다.

재설치·제거·이전 Node CLI 복귀는 [업데이트와 제거 절차](../tool/TUTORIAL.ko.md#업데이트와-제거)를 따르세요. 구 Node 설치기는 `.refactor/lib/src`와 `.refactor/lib/bin`을 교체하므로 실행 전에 두 디렉터리에 사용자 파일이 있는지 확인해야 합니다.

## 실행

첫 실행 전에 [실행 가이드](../tool/TUTORIAL.ko.md#한도-설정과-실행)를 따라 한도를 정하세요. 대상 저장소에서 실행합니다.

```bash
./.refactor/bin/refactor-me version
./.refactor/bin/refactor-me doctor
./.refactor/bin/refactor-me --provider codex --fallback none
```

`doctor`는 진단 기록을 쓰고 모델을 호출해 계정 사용량을 소비합니다. `doctor --no-live-probe`는 모델 호출을 생략하며 세션의 Skill 로딩 여부는 확인하지 않습니다.

<details>
<summary>프로바이더 전환과 탐색 범위 옵션</summary>

```bash
./.refactor/bin/refactor-me --provider codex --fallback claude
./.refactor/bin/refactor-me --target app/web
./.refactor/bin/refactor-me --target app/web --target app/api
```

`--target`은 후보 탐색 범위입니다. 호출부 수정과 검증은 폴더 밖까지 이어질 수 있습니다. [범위와 검증 기준](../tool/README.ko.md#후보-선택)을 확인하세요.

</details>

실행 중에는 원본 checkout을 깨끗하게 유지하세요. CLI는 로컬 커밋과 결과 브랜치를 만들며 merge, push, 배포는 수행하지 않습니다.

## 리포트

실행 후 결과를 확인하세요. 기본 언어는 영어입니다.

```bash
./.refactor/bin/refactor-me report
./.refactor/bin/refactor-me report --lang ko
./.refactor/bin/refactor-me report --json
```

조회할 때 파일을 덮어쓰거나 모델을 호출하지 않습니다. **새 실행부터** 한국어를 사용하려면:

```bash
./.refactor/bin/refactor-me --lang ko
```

리포트에는 파일 통계, diff 미리보기와 전체 텍스트 patch인 `changes.patch`가 있습니다. 종료 상태와 변경을 검토한 뒤 병합하세요. [리포트 형식과 제한사항](../tool/README.ko.md#리포트와-언어), [단계별 Skill 역할](../tool/README.ko.md#skill-설치-상태)은 상세 문서에 있습니다.

## 검증

이 저장소 루트에서 `tool/go`로 이동해 실행하세요.

```bash
cd tool/go
go test -race ./...
go vet ./...
go build -o /private/tmp/refactor-me ./cmd/refactor-me
/private/tmp/refactor-me version --json
```

[로컬 fixture 안내](../tool/fixtures/README.ko.md)는 Go 예제 생성과 선택적 JavaScript 검증을 설명합니다. 로컬 테스트는 설치와 CLI 동작을 확인합니다. 실제 모델의 판단 품질은 별도 실행으로 확인해야 합니다. [검증 범위](../tool/README.ko.md#로컬-개발-검사)를 참고하세요.

## 라이선스

CLI와 문서는 [MIT License](../LICENSE)를 적용하며 저작권자는 2026 soom-kang입니다. 설치된 Go 바이너리에도 저장소의 라이선스가 적용됩니다. 필수 의존성인 sharpen-me Skill은 각자의 MIT 라이선스 파일을 유지합니다. Codex와 Claude Code는 외부 선행 도구로 각 공급자의 약관을 따릅니다.
