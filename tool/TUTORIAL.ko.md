# refactor-me 실행 가이드

[프로젝트](../docs/README.ko.md) · [English](TUTORIAL.md) · [상세 문서](README.ko.md) · [Workflow](WORKFLOW.ko.md)

대상 저장소를 준비한 뒤 아래 5단계를 진행하세요. 각 명령의 실행 위치를 확인하고 예시 경로를 실제 경로로 바꾸세요.

<a id="대상-저장소-준비"></a>

## 1. 대상 저장소 준비

macOS Apple Silicon과 Homebrew를 사용합니다. Formula는 Go를 빌드 의존성으로 관리하며 수동 소스 빌드에는 Go 1.27이 필요합니다. 리팩터링할 저장소에서 확인하세요.

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

## 2. 도구와 전역 Skill 설치

`0.10.0-beta.1`은 릴리스 후보입니다. Homebrew 명령은 tap과 소스 릴리스 게시 후 사용할 수 있습니다. 게시 전에는 [개발 빌드](../docs/README.ko.md#설치)를 사용하세요.

개발 빌드를 사용한다면 아래 모든 명령의 `refactor-me`를 `/private/tmp/refactor-me`로 바꿔 실행하세요.

```sh
brew install soom-kang/refactor-me/refactor-me
refactor-me version --json
npx skills add \
  https://github.com/soom-kang/sharpen-me/tree/v0.9.0-beta.2 \
  --global --skill '*' --agent codex claude-code
refactor-me init --repo /path/to/target-repo
refactor-me doctor --repo /path/to/target-repo --no-live-probe
```

Homebrew는 Go로 CLI를 빌드하고 PATH에서 실행할 수 있게 설치합니다. 외부 `npx` 설치기에는 Node.js가 필요하지만 CLI 실행에는 필요하지 않습니다. Skill의 공통 원본은 `~/.agents/skills`입니다. 이 실행 흐름을 위해 대상 프로젝트에 Skill을 커밋하지 마세요. provider 탐색 경로에 이름이 같고 원본은 다른 Skill이 있으면 실행을 막고 경로를 알려줍니다. 같은 원본으로 해석되는 링크는 허용합니다.

`init`은 선택 사항이며 기존 파일을 덮어쓰지 않고 `.refactor/config.json`을 만듭니다. 바이너리나 Skills를 복사하지 않습니다. 인자 없는 `refactor-me`는 도움말을 표시합니다.

Codex는 선택한 전역 Skill 경로를 사용합니다. Claude에는 실행별 전용 복사본을 `--add-dir`로 전달하고 `--setting-sources project`로 사용자 설정을 계속 제외합니다. provider 호출 전후 Skill 내용이 예상과 다르게 바뀌면 결과 반영을 중단합니다.

디스크 전용 doctor 검사는 진단 기록을 쓰지만 실제 세션의 Skill 로딩을 입증하지는 않습니다. 인증된 provider 세션까지 확인하려면 아래 명령을 별도로 실행하세요. 모델을 호출하고 계정 사용량을 소비합니다.

```sh
refactor-me doctor --repo /path/to/target-repo
```

이 Beta는 Apple 서명·공증을 제공하지 않습니다. macOS가 다운로드한 실행 파일을 차단하면 [Apple의 개별 앱 열기 안내](https://support.apple.com/en-gb/102445)를 따르세요. 시스템 전체 보안 설정을 끄지 마세요.

<a id="한도-설정과-실행"></a>
<a id="set-limits-and-run"></a>

## 3. 실행 한도 설정

`init` 후 `/path/to/target-repo/.refactor/config.json`에서 첫 실행의 `policy.max_commits`를 `1`, `policy.max_wall_clock_min`을 적절한 값(예: `30`)으로 설정하세요. Characterization 테스트 커밋은 이 리팩터링 커밋 한도와 별도로 셉니다.

**전체 금액 예산은 강제하지 않습니다.** Claude 예산 옵션도 실행 전체의 한도가 아닙니다. [설정과 기본값](README.ko.md#설정)을 확인하세요.

## 4. 실행

Codex만 사용하려면 대상 저장소에서 실행하세요.

```bash
refactor-me run --provider codex --fallback none
```

Claude를 대체 프로바이더로 사용하려면:

```bash
refactor-me run --provider codex --fallback claude
```

특정 폴더에서 후보를 찾으려면:

```bash
refactor-me run --target app/web
```

`--repo`를 생략한 `--target`은 현재 디렉터리 기준입니다. 다른 위치에서 실행하려면 `refactor-me run --repo /path/to/target-repo --target app/web`을 사용하세요. 이때 target은 선택한 Git 루트 기준입니다. `--repo`에는 저장소나 그 안의 디렉터리를 지정할 수 있으며 상대 경로는 호출 디렉터리 기준입니다. 실제 경로가 선택한 저장소 밖이면 거부합니다. 호출부 수정과 검증은 저장소 안에서 target 밖까지 이어질 수 있습니다. 실행 중에는 원본 checkout을 수정하지 마세요.

통과한 변경은 `refactor/auto-*` 로컬 브랜치에 반영합니다. 후보 소진, 실행 한도, 반복 실패, 프로바이더 사용 불가, 안전 규칙 위반이 종료 조건입니다.

**종료 코드 `0`도 부분 완료일 수 있습니다.** 다음 단계에서 상태를 확인하세요.

<a id="결과-확인"></a>

## 5. 결과 확인

```bash
refactor-me report
refactor-me report --lang ko
refactor-me report --json
```

실행할 때부터 리포트와 종료 요약을 한국어로 저장하려면:

```bash
refactor-me run --provider codex --fallback none --lang ko
```

언어를 바꿔 조회해도 저장된 `report.md`는 유지합니다. `--lang`은 `run`과 `report`에 적용하며 doctor와 진행 로그는 영어입니다. 번역 대상과 지원하는 JSON 형식은 [리포트와 언어](README.ko.md#리포트와-언어)를 확인하세요.

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
| 전역 Skill 누락 | `~/.agents/skills` 아래 해당 경로를 확인하고 필요하면 전역 설치 명령 재실행 |
| Skill 이름 충돌 | 표시된 provider·프로젝트 경로를 확인하고 사용할 원본 선택. CLI가 파일을 삭제하지 않음 |
| 실행 중 Skill 변경 | 진단 기록 보존 후 카탈로그 수정을 마치고 새 실행 시작 |
| 원본 checkout에 변경 있음 | 작업을 마치거나 별도 보관 후 doctor 재실행 |
| 사용할 기준선 없음 | 명령 실패, 의존성과 빌드 캐시 확인, 탐색이 부족하면 검증 명령 직접 지정 |
| 실행할 후보 없음 | 제외 사유 확인, 변경 없이 끝날 수 있음 |
| 부분 완료 | 다시 실행하기 전에 커밋된 변경과 종료 사유 확인 |
| 안전 정지, 종료 코드 `4` | Worktree와 진단 기록을 보존하고 위반한 불변식 조사 |
| 리포트 JSON 누락 또는 형식 오류 | 원본 JSON이 있으면 복구, 기존 Markdown은 덮어쓰지 않음 |

검증 명령을 직접 지정하는 방법은 [검증 명령](README.ko.md#검증-명령)을 참고하세요. 실행 기록에는 소스 일부와 명령 출력이 포함될 수 있으므로 공유 전에 검토하세요.

## 업데이트와 제거

```sh
brew upgrade soom-kang/refactor-me/refactor-me
refactor-me version --json
refactor-me doctor --repo /path/to/target-repo --no-live-probe
```

업데이트는 모든 프로젝트가 사용하는 실행 파일을 바꿉니다. 먼저 릴리스 노트를 확인하세요. 전역 Skills는 별도로 관리합니다. 고정한 카탈로그를 업데이트하기 전에 로컬 수정 사항을 확인하고 실행 중에는 업데이트하지 마세요.

현재 설정에는 `schema_version: 2`, 리포트에는 `schemaVersion: 3`이 필요합니다. 이전 형식에는 오류를 내며 자동 변환·삭제하지 않습니다. 마이그레이션이나 Node rollback 명령은 없습니다. 이전 설정이 있다면 활성 `.refactor/config.json` 경로 밖에 보관한 뒤 `init`을 실행하고 검토한 설정값을 새 형식에 옮기세요. 과거 실행 기록은 별도로 보관할 수 있지만 현재 CLI로 렌더링할 수는 없습니다. `init` 자체는 기존 파일을 옮기거나 덮어쓰지 않습니다.

정리 가능한 완료 worktree를 제거하려면:

```sh
refactor-me clean --repo /path/to/target-repo
```

부분 완료·미완료·안전 정지 worktree는 조사할 수 있도록 보존합니다. Homebrew 실행 파일을 제거하려면:

```sh
brew uninstall refactor-me
```

제거해도 프로젝트의 설정·리포트·worktree·결과 브랜치와 전역 Skills는 남습니다. CLI의 프로젝트별 `install`·`uninstall` 명령은 더 이상 제공하지 않으며 이전 로컬 실행 파일도 자동 삭제하지 않습니다. `command -v refactor-me`와 `version --json`으로 사용하는 실행 파일을 확인하세요. 과거 프로젝트 파일은 내용을 확인한 뒤 직접 정리하세요.
