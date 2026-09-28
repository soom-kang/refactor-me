# refactor-me 실행 가이드

[프로젝트](../docs/README.ko.md) · [English](TUTORIAL.md) · [상세 문서](README.ko.md) · [Workflow](WORKFLOW.ko.md)

대상 저장소를 준비한 뒤 아래 5단계를 진행하세요. 각 명령의 실행 위치를 확인하고 예시 경로를 실제 경로로 바꾸세요.

<a id="대상-저장소-준비"></a>

## 1. 대상 저장소 준비

macOS Apple Silicon을 사용합니다. Go 1.27은 소스에서 빌드할 때만 필요합니다. 리팩터링할 저장소에서 확인하세요.

```bash
cd /path/to/target-repo
git rev-parse --show-toplevel
git status --short
```

기존 작업을 마치거나 별도로 보관하세요. 미추적 파일을 포함해 원본 checkout이 깨끗해야 합니다.

프로젝트 의존성을 설치하고 빌드와 테스트를 한 번 실행하세요. 필요한 캐시를 준비하고 기존 실패를 확인하는 단계입니다.

사용할 프로바이더의 인증 상태를 확인합니다.

```bash
codex login status
# Claude Code를 사용하는 경우:
claude auth status
```

프로바이더 호출에는 네트워크가 필요하며 해당 계정의 사용량을 소비합니다.

<a id="도구와-skill-설치"></a>

## 2. 도구와 Skill 설치

[`v0.9.20-beta.1` 릴리스](https://github.com/soom-kang/refactor-me/releases/tag/v0.9.20-beta.1)에서 압축파일과 checksum 파일을 받으세요. 터미널에서 실행합니다.

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

압축파일에는 `refactor-me`, `LICENSE`, `INSTALL.md`, `BUILD-INFO.txt`가 들어갑니다. 설치 전에 `INSTALL.md`를 읽고 출력 버전을 확인하세요. `SHA256SUMS`는 내려받은 파일의 변경을 확인할 뿐 게시자 신원을 증명하지는 않습니다. 첫 Beta는 서명·공증하지 않으므로 macOS에서 차단되거나 경고가 나올 수 있습니다. 필요하면 [Apple의 개별 앱 열기 안내](https://support.apple.com/en-gb/102445)를 따르세요. 시스템 전체의 보안 설정은 끄지 마세요. 개발용 소스 빌드는 [README 설치 안내](../docs/README.ko.md#설치)를 참고하세요.

대상 저장소에 필수 의존성인 sharpen-me Skill 8개를 설치하세요.

```bash
cd /path/to/target-repo
npx skills add soom-kang/sharpen-me --skill '*' --agent codex claude-code
```

Project 범위를 선택하세요. Skill 디렉터리 8개, 에이전트 링크와 `skills-lock.json`을 검토하고 커밋하세요. Worktree는 기준 커밋의 파일을 읽습니다. 관련 없는 파일은 함께 stage하지 마세요.

```bash
git add .agents .claude skills-lock.json && git commit
```

두 디렉터리를 모두 커밋해야 합니다. 실체 파일은 `.agents/skills/<name>/`에 놓이고 `.claude/skills/<name>`은 그곳을 가리키는 심볼릭 링크입니다. 한쪽만 커밋하면 worktree에서 링크가 아무것도 가리키지 못합니다.

설치된 도구를 확인합니다.

```bash
./.refactor/bin/refactor-me version
./.refactor/bin/refactor-me help
./.refactor/bin/refactor-me doctor --no-live-probe
./.refactor/bin/refactor-me doctor
```

Doctor는 `.refactor/runs/`에 진단 기록을 쓰고 모델을 호출합니다. 실행을 막는 실패를 해결하세요. `doctor --no-live-probe`는 디스크만 검사하며 세션의 Skill 로딩은 확인하지 않습니다.

<a id="한도-설정과-실행"></a>

## 3. 실행 한도 설정

`.refactor/config.json`에서 첫 실행의 `policy.max_commits`를 `1`, `policy.max_wall_clock_min`을 적절한 값(예: `30`)으로 설정하세요. Characterization 테스트 커밋은 이 리팩터링 커밋 한도와 별도로 셉니다.

**전체 금액 예산은 강제하지 않습니다.** Claude 예산 옵션도 실행 전체의 한도가 아닙니다. [설정과 기본값](README.ko.md#설정)을 확인하세요.

## 4. 실행

Codex만 사용하려면 대상 저장소에서 실행하세요.

```bash
./.refactor/bin/refactor-me --provider codex --fallback none
```

Claude를 대체 프로바이더로 사용하려면:

```bash
./.refactor/bin/refactor-me --provider codex --fallback claude
```

특정 폴더에서 후보를 찾으려면:

```bash
./.refactor/bin/refactor-me --target app/web
```

`--target`은 현재 디렉터리 기준입니다. 호출부 수정과 검증은 대상 밖까지 이어질 수 있습니다. 실행 중에는 원본 checkout을 수정하지 마세요.

통과한 변경은 `refactor/auto-*` 로컬 브랜치에 반영합니다. 후보 소진, 실행 한도, 반복 실패, 프로바이더 사용 불가, 안전 규칙 위반이 종료 조건입니다.

**종료 코드 `0`도 부분 완료일 수 있습니다.** 다음 단계에서 상태를 확인하세요.

<a id="결과-확인"></a>

## 5. 결과 확인

```bash
./.refactor/bin/refactor-me report
./.refactor/bin/refactor-me report --lang ko
./.refactor/bin/refactor-me report --json
```

실행할 때부터 리포트와 종료 요약을 한국어로 저장하려면:

```bash
./.refactor/bin/refactor-me --provider codex --fallback none --lang ko
```

언어를 바꿔 조회해도 저장된 `report.md`는 유지합니다. `--lang`은 `run`과 `report`에 적용하며 doctor와 진행 로그는 영어입니다. 번역 대상과 JSON 호환성은 [리포트와 언어](README.ko.md#리포트와-언어)를 확인하세요.

리포트에서 커밋한 변경, 제외한 후보, 검증 결과, 사용량과 worktree 경로를 확인하세요. 미보고 비용은 0이 아니며 일부만 집계한 금액은 최소 금액입니다.

파일 통계와 diff 미리보기를 읽고 `.refactor/runs/<id>/changes.patch`에서 전체 변경을 확인하세요. 비교는 시작·최종 반영 커밋의 고정 OID를 사용합니다. 비교 실패는 실행 결과와 별개이므로 사유를 확인하세요.

Git에서 다시 확인하려면 `codeComparison`의 시작·최종 반영 커밋 전체 OID를 사용합니다.

```bash
git log --oneline <base-commit>..<published-commit>
git diff <base-commit> <published-commit>
```

병합 전에는 diff를 검토하고 생략된 서비스, 브라우저, 통합 검사를 실행하세요. 후보 검증은 변경 영역을 선택하고 루트 영역이 있으면 함께 검사합니다. 모든 영역의 통과를 보장하지는 않습니다. 기준선과 같은 실패도 통과로 세지 않습니다.

단계별 검사는 [Workflow](WORKFLOW.ko.md)를 참고하세요. `handoff.md`는 중간 기록이며 최종 결과는 리포트에서 확인합니다.

## 중단된 실행 확인

| 증상 | 다음 조치 |
| --- | --- |
| Skill 누락 | 해당 Skill을 Project 범위로 설치하고 에이전트 링크 확인 |
| `skill-worktree` 실패 | 기준 커밋 checkout에 필요한 Skill이 없음. `.agents`와 `.claude`를 함께 커밋. `.claude` 항목은 `.agents`로 가는 심볼릭 링크이므로 한쪽만으로는 부족 |
| 원본 checkout에 변경 있음 | 작업을 마치거나 별도 보관 후 doctor 재실행 |
| 사용할 기준선 없음 | 명령 실패, 의존성과 빌드 캐시 확인, 탐색이 부족하면 검증 명령 직접 지정 |
| 실행할 후보 없음 | 제외 사유 확인, 변경 없이 끝날 수 있음 |
| 부분 완료 | 다시 실행하기 전에 커밋된 변경과 종료 사유 확인 |
| 안전 정지, 종료 코드 `4` | Worktree와 진단 기록을 보존하고 위반한 불변식 조사 |
| 리포트 JSON 누락 또는 형식 오류 | 원본 JSON이 있으면 복구, 기존 Markdown은 덮어쓰지 않음 |

검증 명령을 직접 지정하는 방법은 [검증 명령](README.ko.md#검증-명령)을 참고하세요. 실행 기록에는 소스 일부와 명령 출력이 포함될 수 있으므로 공유 전에 검토하세요.

## 업데이트와 제거

업데이트하려면 `NEW_VERSION`에 **실제로 게시된 새 버전**을 넣고 새 디렉터리에 내려받으세요. 새 터미널에서도 그대로 시작할 수 있으며 hash나 바이너리 버전이 다르면 설치 전에 중단합니다. 업데이트가 확인될 때까지 이전에 검증한 압축파일을 보관하세요.

```bash
NEW_VERSION=0.9.20-beta.1  # 실제 게시된 새 버전으로 변경
RELEASE_DIR="$(mktemp -d)"
(
set -e
RELEASE_URL="https://github.com/soom-kang/refactor-me/releases/download/v${NEW_VERSION}"
curl -fL "$RELEASE_URL/refactor-me_${NEW_VERSION}_darwin_arm64.zip" -o "$RELEASE_DIR/refactor-me_${NEW_VERSION}_darwin_arm64.zip"
curl -fL "$RELEASE_URL/SHA256SUMS" -o "$RELEASE_DIR/SHA256SUMS"
cd "$RELEASE_DIR"
shasum -a 256 -c SHA256SUMS
unzip -q "refactor-me_${NEW_VERSION}_darwin_arm64.zip"
cat INSTALL.md
RELEASE_INFO="$(./refactor-me version --json)"
printf '%s\n' "$RELEASE_INFO" | grep -Fq "\"version\": \"$NEW_VERSION\""
printf '%s\n' "$RELEASE_INFO" | grep -Fq '"platform": "darwin"'
printf '%s\n' "$RELEASE_INFO" | grep -Fq '"arch": "arm64"'
./refactor-me install /path/to/target-repo
)
```

재설치는 소유가 확인된 Go 실행 파일을 교체하고 `.refactor/config.json`, 실행 기록과 `last-run.json`을 보존합니다. Node 전환 시 식별된 shim만 교체하며 사용자 파일이나 식별되지 않은 파일은 보존하고 경로를 알립니다. 소유를 확인할 수 없는 명령이 있으면 교체 전에 중단합니다. 보존된 파일을 삭제하기 전에 직접 확인하세요.

대상 저장소의 카탈로그를 업데이트하기 전에 로컬 Skill 수정 사항을 검토하세요. 카탈로그 관리는 [sharpen-me 문서](https://github.com/soom-kang/sharpen-me)를 참고하고 검토한 변경을 커밋하세요.

완료된 worktree를 정리하려면 대상 저장소에서 실행합니다.

```bash
./.refactor/bin/refactor-me clean
```

부분 완료, 미완료, 안전 정지 상태의 worktree는 보존합니다. Go 도구를 제거하려면 내려받아 검증한 릴리스 실행 파일을 사용하세요. `RELEASE_DIR`는 압축을 푼 디렉터리의 절대 경로로 바꾸세요.

```bash
RELEASE_DIR=/absolute/path/to/verified/refactor-me-release
test -x "$RELEASE_DIR/refactor-me"
"$RELEASE_DIR/refactor-me" uninstall /path/to/target-repo
```

제거 명령은 소유가 확인된 Go 바이너리만 삭제합니다. 설정과 실행 기록을 보존하며 Skill 카탈로그, 결과 브랜치와 worktree는 제거하지 않습니다. 사용자 정의 명령은 도구 설치 디렉터리 밖에 보관하세요.

이전 Node CLI로 돌아가려면 Node.js 24를 준비하고 검증된 `v0.8.8-beta.1` 소스를 받으세요. 예상 commit은 `fa845c98fba01f87466531ae50c5f1d671f0392f`입니다. 구 Node 설치기는 `.refactor/lib/src`와 `.refactor/lib/bin`을 교체하므로, 전환 후 library 파일이 남아 있으면 아래 명령은 중단합니다. 해당 파일은 별도로 확인하고 해결하세요. 검사를 통과하기 위해 임의로 지우지 마세요. `RELEASE_DIR`는 검증한 Go 릴리스 디렉터리의 절대 경로로 바꾸세요. `.refactor/config.json`과 `.refactor/runs/`는 유지합니다.

```bash
RELEASE_DIR=/absolute/path/to/verified/refactor-me-release
test -x "$RELEASE_DIR/refactor-me"
ROLLBACK_SOURCE="$(mktemp -d)/refactor-me-v0.8.8-beta.1"
git clone --quiet --depth 1 --branch v0.8.8-beta.1 https://github.com/soom-kang/refactor-me.git "$ROLLBACK_SOURCE"
(
set -e
TARGET=/path/to/target-repo
test "$(git -C "$ROLLBACK_SOURCE" rev-parse HEAD)" = fa845c98fba01f87466531ae50c5f1d671f0392f
if [ -d "$TARGET/.refactor/lib" ] && [ -n "$(find "$TARGET/.refactor/lib" -mindepth 1 -print -quit)" ]; then
  echo 'Stop: inspect retained .refactor/lib files before Node rollback' >&2
  exit 2
fi
"$RELEASE_DIR/refactor-me" uninstall "$TARGET"
node "$ROLLBACK_SOURCE/tool/install.mjs" "$TARGET"
"$TARGET/.refactor/bin/refactor-me" version
"$TARGET/.refactor/bin/refactor-me" doctor --no-live-probe
)
```

Go 제거 후 Node 설치기나 `doctor`가 실패하면 검증된 Go 압축파일을 보관한 상태에서 그 실행 파일의 `install` 명령으로 Go CLI를 복구하세요. 이때 소유를 확인할 수 없는 명령이나 파일 오류가 나오면 부분적으로 설치된 Node 파일을 먼저 조사하고, 수동으로 덮어쓰지 마세요. 버전 출력만으로 보존된 설정이 이전 Node CLI에서 작동한다고 판단하지 않습니다.
