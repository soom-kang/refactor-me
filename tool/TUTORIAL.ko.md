# refactor-me 실행 가이드

[프로젝트](../docs/README.ko.md) · [English](TUTORIAL.md) · [상세 문서](README.ko.md) · [Workflow](WORKFLOW.ko.md)

대상 저장소를 준비한 뒤 아래 5단계를 진행하세요. 각 명령의 실행 위치를 확인하고 예시 경로를 실제 경로로 바꾸세요.

<a id="대상-저장소-준비"></a>

## 1. 대상 저장소 준비

macOS와 Node.js 24 이상을 사용합니다. 리팩터링할 저장소에서 확인하세요.

```bash
cd /path/to/target-repo
node --version
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

[Beta 설치 안내](../docs/README.ko.md#설치)에 따라 clone한 refactor-me checkout에서 실행하세요.

```bash
node tool/install.mjs /path/to/target-repo
```

대상 저장소에 필수 의존성인 sharpen-me Skill 8개를 설치하세요.

```bash
cd /path/to/target-repo
npx skills add soom-kang/sharpen-me --skill '*' --agent codex claude-code
```

Project 범위를 선택하세요. Skill 디렉터리 8개, 에이전트 링크와 `skills-lock.json`을 검토하고 커밋하세요. Worktree는 기준 커밋의 파일을 읽습니다. 관련 없는 파일은 함께 stage하지 마세요.

설치된 도구를 확인합니다.

```bash
./.refactor/bin/refactor-me version
./.refactor/bin/refactor-me help
./.refactor/bin/refactor-me doctor
```

Doctor는 `.refactor/runs/`에 진단 기록을 쓰고 모델을 호출합니다. 실행을 막는 실패를 해결하세요. `doctor --no-live-probe`는 디스크만 검사하며 세션의 Skill 로딩은 확인하지 않습니다.

<a id="한도-설정과-실행"></a>

## 3. 실행 한도 설정

`.refactor/config.json`에서 첫 실행의 `policy.max_commits`를 `1`로 설정하고 경과 시간 한도를 정하세요. Characterization 테스트 커밋은 이 리팩터링 커밋 한도와 별도로 셉니다.

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
| `skill-worktree` 실패 | 프로젝트 Skill 파일을 검토하고 커밋한 뒤 재실행 |
| 원본 checkout에 변경 있음 | 작업을 마치거나 별도 보관 후 doctor 재실행 |
| 사용할 기준선 없음 | 명령 실패, 의존성과 빌드 캐시 확인, 탐색이 부족하면 검증 명령 직접 지정 |
| 실행할 후보 없음 | 제외 사유 확인, 변경 없이 끝날 수 있음 |
| 부분 완료 | 다시 실행하기 전에 커밋된 변경과 종료 사유 확인 |
| 안전 정지, 종료 코드 `4` | Worktree와 진단 기록을 보존하고 위반한 불변식 조사 |
| 리포트 JSON 누락 또는 형식 오류 | 원본 JSON이 있으면 복구, 기존 Markdown은 덮어쓰지 않음 |

검증 명령을 직접 지정하는 방법은 [검증 명령](README.ko.md#검증-명령)을 참고하세요. 실행 기록에는 소스 일부와 명령 출력이 포함될 수 있으므로 공유 전에 검토하세요.

## 업데이트와 제거

갱신한 refactor-me checkout에서 같은 설치 명령을 실행합니다.

```bash
node tool/install.mjs /path/to/target-repo
```

재설치는 실행 파일을 교체하고 `.refactor/config.json`, 실행 기록과 `last-run.json`을 보존합니다. 이전 설치기가 생성한 명령은 제거하며 호환 별칭은 만들지 않습니다. 이전 명령에 사용자 수정이 있으면 교체 전에 중단하므로 먼저 확인하세요.

대상 저장소의 카탈로그를 업데이트하기 전에 로컬 Skill 수정 사항을 검토하세요. 카탈로그 관리는 [sharpen-me 문서](https://github.com/soom-kang/sharpen-me)를 참고하고 검토한 변경을 커밋하세요.

완료된 worktree를 정리하려면 대상 저장소에서 실행합니다.

```bash
./.refactor/bin/refactor-me clean
```

부분 완료, 미완료, 안전 정지 상태의 worktree는 보존합니다. 도구를 제거하려면 소스 checkout에서 실행합니다.

```bash
node tool/install.mjs /path/to/target-repo --uninstall
```

제거 명령은 `.refactor/lib`와 `.refactor/bin`을 삭제합니다. 설정과 실행 기록을 보존하며 Skill 카탈로그, 결과 브랜치와 worktree는 제거하지 않습니다. 사용자 정의 명령은 도구 설치 디렉터리 밖에 보관하세요.
