# refactor-me 상세 문서

[프로젝트](../docs/README.ko.md) · [English](README.md) · [실행 가이드](TUTORIAL.ko.md) · [Workflow](WORKFLOW.ko.md) · [변경 이력](CHANGELOG.md)

[설정](#설정) · [검증 명령](#검증-명령) · [리포트](#리포트와-언어) · [Skill](#skill-설치-상태)

설치는 [실행 가이드](TUTORIAL.ko.md)를 따라 진행하세요.

## 명령과 저장소 선택

| 명령 | 용도 |
| --- | --- |
| `help` 또는 인자 없음 | 실행을 시작하지 않고 사용법 표시 |
| `version [--json]` | 빌드 버전·Go 런타임·플랫폼·아키텍처·실행 파일 경로·commit 출처 표시 |
| `init` | 기존 설정을 덮어쓰지 않고 선택적 프로젝트 설정 생성 |
| `doctor [--no-live-probe]` | 저장소·provider·전역 Skills 검사. 기본값은 모델 호출 포함 |
| `run` | 자동 리팩터링 루프 시작 |
| `report [--json] [--lang en\|ko]` | 지원하는 최신 저장 리포트 조회 |
| `clean` | 정리 가능한 완료 worktree 제거 |

`init`, `doctor`, `run`, `report`, `clean`에는 `--repo <path>`를 지정할 수 있습니다. 상대 경로는 호출 디렉터리에서 해석하며 해당 경로의 Git 루트를 찾습니다. 생략하면 현재 디렉터리의 Git 루트를 사용합니다. `help`와 `version`은 Git 밖에서도 동작합니다. 설치는 Homebrew가 담당하며 프로젝트별 `install`·`uninstall` 명령은 없습니다.

설정·잠금·실행 기록은 선택한 저장소에 속하며 Homebrew 실행 파일 경로와 독립적입니다. 새 설치에는 `.refactor/bin`이나 `.refactor/lib` 파일이 필요하지 않습니다.

## 설정

`refactor-me init --repo <path>`는 `.refactor/config.json`이 없으면 생성하고 기존 파일은 덮어쓰지 않습니다. 초기화는 선택 사항이며 설정 파일이 없으면 내장 기본값을 사용합니다. 실행 기본값은 `go/internal/surface/config.go`와 `go/internal/controller`, 초기화 템플릿은 [config.default.json](config.default.json)에 있습니다. 설정에는 `schema_version: 2`가 필요합니다. 기존 파일의 버전이 없거나 지원되지 않으면 자동 이관하지 않고 오류를 냅니다.

| 설정 | 기본값 | 동작 |
| --- | --- | --- |
| `workspace.branch_prefix` | `refactor/auto-` | 결과 브랜치 접두사 |
| `workspace.worktree_parent` | 빈 값 | `~/.cache/refactor-me` 사용, 경로를 지정하면 우선 적용 |
| `workspace.keep_worktree` | `true` | Worktree 보존, `false`이면 `DONE` 또는 `NO_CHANGES` 후 제거 |
| `agents.primary` / `agents.fallback` | `codex` / `claude` | 시작 프로바이더 순서 |
| `agents.<name>.enabled` | `true` | 프로바이더 활성화 |
| `agents.<name>.bin` | 프로바이더 이름 | CLI 실행 파일 |
| `agents.<name>.model` | 빈 값 | 프로바이더 CLI 기본 모델 사용 |
| `agents.<name>.timeout_sec` | `1800` | 프로바이더 호출 제한 시간, doctor는 더 짧은 검사 시간 사용 |
| `agents.<name>.effort_by_phase` | 빈 값 | 단계별 추론 수준 지정 |
| `agents.<name>.effort` | 미지정 | 기본 추론 수준 표 대체, 단계별 값이 있으면 그 값 우선 |
| `agents.claude.max_budget_usd` | `0` | 양수이면 Claude의 호출별 예산 옵션으로 전달 |
| `policy.allowed_risks` | `L0_LOW`, `L1_MODERATE`, `L2_HIGH` | 허용 위험도, `UNKNOWN`과 `L3_CRITICAL`은 계속 제외 |
| `policy.unknown_risk` | `set_aside` | `UNKNOWN` 위험도 처리 방식. `deep_check`는 준비 상태가 `NEEDS_EVIDENCE`인 `UNKNOWN` 후보를 제외하지 않고 deep check로 보냅니다. 그 밖의 `UNKNOWN`은 계속 제외합니다 |
| `policy.max_cycles` / `policy.max_commits` | `25` / `20` | 사이클과 리팩터링 커밋 한도 |
| `policy.max_wall_clock_min` | `180` | 작업 단위 사이에 확인하는 경과 시간 한도 |
| `policy.max_consecutive_failures` | `3` | 연속 후보 실패 한도 |
| `policy.max_attempts_per_fingerprint` | `2` | 후보 fingerprint별 시도 한도 |
| `policy.empty_audits_to_stop` | `2` | 종료에 필요한 연속 빈 audit 횟수 |
| `policy.max_files_per_candidate` | `8` | 후보 파일 수 한도 |
| `policy.max_changed_lines` | `600` | 기본 변경 줄 수 한도, 파일 분할에는 별도 계산 적용 |
| `policy.max_audit_candidates` | `8` | 후보 선택 수 한도 |
| `policy.cooldown_minutes` | `20` | 리소스 소진 후 프로바이더 대기 시간 |
| `policy.auto_characterization` | `true` | 필요한 경우 characterization 테스트 허용 |
| `policy.cross_provider_review` | `true` | 다른 프로바이더의 리뷰 우선 |
| `policy.extra_forbidden_globs` | 빈 값 | 기본 금지 목록에 경로 추가 |

기본 추론 수준은 다음과 같습니다.

- `high`: audit, deep check, 실행, 리뷰
- `medium`: preflight, characterization, 재조사
- `low`: doctor, handoff

프로바이더 설정으로 바꿀 수 있으며 단계별 값이 우선합니다.

`verification`은 설정 형식에 남아 있습니다. 현재 루프는 검증 명령을 고를 때 `verification.locked`를 사용하지 않습니다. 직접 지정하려면 아래의 `.refactor/commands.json`을 사용하세요.

## 후보 선택

Audit은 다음 네 종류의 후보를 찾습니다.

| 분류 | 후보 |
| --- | --- |
| `DEAD_CODE` | 도달 가능한 진입점에서 더는 참조하지 않는 코드 |
| `COMPATIBILITY_REMOVAL` | 사용하지 않는 호환성 코드나 마이그레이션 코드 |
| `DEDUPLICATION` | 세 곳 이상에서 반복되는 로직 |
| `LARGE_COMPONENT_SPLIT` | 기존 경계를 따라 나눌 수 있는 500줄 초과 파일 |

기존 진입점과 공개 인터페이스의 반환값, 부수 효과, 순서, 오류, 화면 출력과 저장 데이터 형태를 보존합니다. 버그 수정과 추측에 따른 개선은 제외합니다.

`--target <dir>`은 여러 번 지정할 수 있습니다. `--repo`를 지정하면 선택한 Git 루트, 생략하면 현재 디렉터리 기준입니다. 심볼릭 링크를 포함한 실제 경로가 선택한 저장소 안에 있어야 합니다.

- 후보 탐색 범위만 제한합니다. 호출부와 도달성은 저장소 전체에서 확인합니다.
- 후보는 대상과 관련이 있어야 하며 변경 결과에 대상 파일이 포함되어야 합니다. 잘못된 경로나 추적 중인 소스 파일이 없는 대상은 실행을 막습니다.
- 후보 검증은 변경 영역을 선택하고 루트 영역이 있으면 함께 검사합니다. [검증 범위](#검증-명령)를 확인하세요.

## 실행과 안전 검사

Audit → deep check → 필요한 characterization → preflight → 실행 → 검증 → 독립 리뷰 순서입니다. 변경을 커밋하면 다시 조사합니다.

원본 저장소 밖에 detached worktree를 만듭니다. 의존성 디렉터리나 로컬 환경 파일 등 gitignore 대상 빌드 입력도 복사할 수 있습니다. **Worktree와 실행 기록에도 원본과 같은 접근 통제를 적용하세요.**

컨트롤러는 고정된 작업 명세와 diff를 비교합니다. 금지 경로, 허용 파일, 변경 규모와 분류별 크기 규칙, 테스트 무결성, 이전 tree hash를 확인합니다. 원본 checkout과 승인한 worktree 상태가 바뀌었는지도 검사합니다. 거절한 후보는 실행용 worktree에서 되돌립니다.

CLI는 로컬 커밋과 결과 브랜치를 만듭니다. merge, push, 배포와 별도 의존성 설치 단계는 없습니다. 기존 검증 명령이나 빌드 도구는 네트워크를 사용하고 캐시를 채울 수 있습니다.

`sharpen-cold-review`는 별도 세션에서 실행합니다. 두 프로바이더가 사용 가능하고 교차 리뷰가 켜져 있으면 구현하지 않은 프로바이더를 우선합니다.

쿼터나 인증 실패 시 같은 단계에서 프로바이더를 바꿀 수 있습니다. 새 세션에는 해당 단계의 입력과 필요한 고정 작업 명세를 전달합니다. `handoff.md`는 전환 기록이며 최종 결과는 리포트에 있습니다.

## Skill 설치 상태

[가이드](TUTORIAL.ko.md#도구와-skill-설치)의 고정한 sharpen-me 카탈로그에서 필수 Skill 8개를 전역 설치하세요. 공통 원본은 `~/.agents/skills`이며 프로젝트의 복사본으로 누락된 전역 Skill을 대신할 수 없습니다.

CLI는 각 Skill의 실제 디렉터리를 찾고 `SKILL.md`와 참조 자료를 확인한 뒤 실제 경로와 전체 내용의 SHA-256을 기록합니다. Skill 디렉터리 자체는 심볼릭 링크일 수 있지만 내부 링크가 해당 실제 디렉터리 밖으로 나가면 거부합니다. 끊어진 링크와 지원하지 않는 파일 구조도 오류입니다. 프로젝트·provider 탐색 경로의 동명 Skill이 서로 다른 원본이면 실행을 막습니다. 같은 원본으로 해석되는 링크는 허용하며 사용자 파일을 삭제하지 않습니다.

| Provider | Skill 전달 방식 |
| --- | --- |
| Codex | 전역 탐색을 사용하고 단계별 프롬프트에 선택한 Skill의 절대 경로 명시 |
| Claude Code | 실행별 전용 `.claude/skills` 복사본을 `--add-dir`로 제공. `--setting-sources project`로 사용자 설정 제외 유지 |

필수 Skill을 대상 저장소의 기준 커밋에 넣을 필요가 없습니다. Claude 전용 디렉터리에는 선택한 Skills와 참조 자료만 담으며 `--add-dir`에 사용자 홈 전체를 제공하지 않습니다. provider 호출 전후 내용 hash를 확인하고 예상하지 못한 변경이 있으면 해당 결과를 반영하기 전에 안전 정지합니다.

Doctor는 전역 원본과 전달 경로를 검사합니다. 기본적으로 모델을 호출해 사용량을 소비하고 진단 기록을 씁니다. `--no-live-probe`는 파일·설정만 검사하며 실제 세션의 로딩을 입증하지는 않습니다. 로컬 fixture 검증과 실제 provider 검증은 별도 근거입니다. live probe는 provider가 여덟 Skill을 모두 볼 수 있다고 응답해야 통과합니다. 이 응답은 세션의 자기보고이며 모든 파일을 실제로 읽었다는 독립 증명은 아닙니다. 파일 경로와 hash는 별도로 측정합니다.

| Skill | 사용 시점 | 결과 |
| --- | --- | --- |
| `sharpen-clarify` | Audit과 characterization의 범위를 정할 때 | 작업 범위와 확인할 계약 |
| `sharpen-review` | Audit 또는 deep check에서 후보를 검토할 때 | 저장소 근거를 갖춘 발견 사항 |
| `sharpen-challenge` | Audit, deep check, characterization, preflight에서 가정을 검토할 때 | 근거가 있는 반론과 확인 방법 |
| `sharpen-assess` | Audit에서 변경 위험도를 판단할 때 | 위험도 분류, 근거가 부족한 후보 제외 |
| `sharpen-refine` | 승인된 작업 명세를 실행할 때 | 범위 안의 변경 또는 변경하지 않은 이유 |
| `sharpen-cold-review` | 별도 세션에서 구현을 검토할 때 | 리뷰 판정과 발견 사항 |
| `sharpen-brief` | 쿼터나 인증 문제로 프로바이더를 전환할 때 | 전환 시점의 상태 요약 |
| `sharpen-dedupe` | 중복 제거 후보를 검증하거나 실행할 때 | 중복 분석과 범위 안의 통합 |

루프가 단계별 Skill 허용 목록, 출력 스키마와 권한을 정합니다. 모델과 추론 수준은 루프 설정을 따르며 Skill의 권고로 바뀌지 않습니다.

실행 리포트에는 Skill의 실제 경로·내용 hash, CLI·provider 버전과 provider별 전달 경로를 기록합니다. 전역 카탈로그 업데이트 전후의 실행을 비교할 때 이 기록을 사용하세요.

## 검증 명령

프로젝트 영역을 최대 세 단계 깊이까지 탐색합니다.

| 기준 파일 | 검토하는 명령 |
| --- | --- |
| `package.json` | 감지한 패키지 관리자의 typecheck, lint, test, build 스크립트 |
| `go.mod` | `go vet`, `go test -count=1`, `go build` |
| `pyproject.toml` | Ruff, mypy, pytest와 적용 가능한 선언된 실행기 |
| `Cargo.toml` | `cargo check`, `cargo test --lib` |
| Gradle JVM 빌드 | `testClasses`, `test` |
| `pom.xml` | `-B test-compile`, `-B test` |
| `Makefile` | 다른 명령이 담당하지 않는 관련 target |

Gradle 탐색에는 JVM 플러그인이 필요합니다. 실행 가능한 Gradle 또는 Maven wrapper가 PATH 실행 파일보다 우선하며 상위 JVM 빌드가 하위 모듈을 담당합니다.

브라우저, E2E, 외부 서비스가 필요한 것으로 감지한 검사는 제외하고 리포트에 기록합니다.

검증 명령을 직접 지정하려면 대상 저장소에 `.refactor/commands.json`을 만드세요.

```json
{
  "locked": true,
  "commands": [
    {
      "id": ".:T2:node:test",
      "area": ".",
      "tier": "T2",
      "name": "test",
      "argv": ["node", "--test"],
      "cwd": ".",
      "timeoutMs": 180000,
      "source": "operator-defined"
    }
  ]
}
```

ID는 고유해야 합니다. `area`와 `cwd`는 저장소 상대 경로, `argv`는 인자 배열, `tier`는 `T1`, `T2`, `T3` 중 하나를 사용하세요. Locked 파일은 자동 탐색을 대체합니다. 실행 전에 명령을 검토하세요. 컨트롤러가 worktree에서 실행합니다.

### 기준선과 후보 검증

기준선은 발견한 영역의 선택된 명령을 실행하고 통과, 해석 가능한 실패, 실행 불가, 유효한 signature가 없는 실패를 구분합니다. **통과 신호가 하나도 없으면 중단합니다.**

후보 수정 후에는 경로별로 가장 깊은 변경 영역을 선택하고 루트 영역이 있으면 함께 검사합니다. 다음 규칙을 적용합니다.

- 기준선의 `TIMEOUT`, `UNRUNNABLE`, `OPAQUE` 명령은 후보 검증에서 제외합니다.
- 해석 가능한 기존 실패에는 새 오류가 없어야 하며 통과했던 명령은 계속 통과해야 합니다. 첫 실패에서 검증을 멈추고 후보를 거절합니다.
- 기존과 같은 실패도 통과로 집계하지 않습니다. 결과가 저장소 모든 영역의 검증 통과를 뜻하지는 않습니다.

## 리포트와 언어

### 언어와 저장 파일

`--lang en|ko`는 `run`과 `report`에 적용하며 기본값은 `en`입니다. 종료 요약, `report.md`, handoff 고정 안내를 번역합니다. 진행 로그와 doctor 출력은 영어이며 모델 설명, 오류, 경로, 커밋 제목은 원문을 유지합니다.

실행마다 `report.md`와 `report.json`을 하나씩 저장합니다. `report --lang ko`는 최신 JSON을 읽어 출력하며 파일을 바꾸지 않습니다. 현재 리포트는 `schemaVersion: 3`을 사용합니다. JSON 누락·형식 오류·지원하지 않는 과거 버전에는 오류를 내고 기존 파일을 보존합니다. 자동 이관하지 않습니다. `--json`은 지원하는 리포트의 저장된 JSON 바이트를 그대로 출력하며 언어와 무관합니다.

### 코드 비교

원본 저장소의 `state.baseOid`와 `state.publishedOid`를 비교합니다. Characterization 커밋을 포함한 최종 변경을 사용하므로 worktree를 정리한 뒤에도 비교할 수 있습니다.

- 미리보기: 파일 통계와 문맥 3줄. 완전한 줄 단위로 최대 200줄 또는 UTF-8 32 KiB.
- `changes.patch`: 전체 텍스트 patch. 바이너리 본문은 생략하고 바이너리와 파일 모드 메타데이터는 유지합니다. 외부 diff와 textconv는 사용하지 않습니다.

`report.json`의 선택 필드 `codeComparison`에는 다음 값을 저장합니다.

| 필드 | 내용 |
| --- | --- |
| `status` | `AVAILABLE`, `NO_CHANGES`, `UNAVAILABLE` |
| `baseCommit`, `resultCommit` | 기록된 전체 OID. 반영한 커밋이 없으면 `resultCommit`은 null |
| `files` | 경로, 이름 변경 전 경로, Git 상태, 이전·이후 모드, 추가·삭제 줄 수와 바이너리 여부 |
| `totals` | 파일 수, 텍스트 추가·삭제 줄 수, 바이너리 파일 수. 바이너리 파일의 개별 줄 수는 null |
| `patchFile` | 실행 디렉터리 기준 `changes.patch`. 저장한 patch가 없으면 null |
| `preview`, `truncated` | 저장한 diff 일부와 생략 여부 |
| `error` | 수집·저장 오류 원문. 오류가 없으면 null |

| 비교 상황 | 결과 |
| --- | --- |
| 반영한 커밋 없음 | `NO_CHANGES`, patch 없음 |
| 반영한 커밋의 두 tree가 같음 | 빈 patch |
| Git 객체 누락 또는 patch 저장 실패 | `UNAVAILABLE`과 사유 기록. 실행 상태와 종료 코드 유지 |
| 지원하지 않는 리포트 schema | 오류, 저장 파일 보존 |

조회할 때 비교를 다시 수집하지 않습니다. `accepted.patch`는 리뷰 전 스냅샷으로 거절한 후보에도 남을 수 있습니다. 커밋된 변경은 최종 비교에서 확인하세요.

### 사용량과 종료 코드

실패한 호출과 스키마 복구 프로세스도 사용량에 포함합니다. 미보고 비용은 0이 아니며 일부 비용만 있는 합계는 최소 금액입니다.

**전체 금액 예산은 강제하지 않습니다.** Claude 예산 옵션은 프로바이더 설정이며 실행 전체의 한도가 아닙니다. 사이클, 커밋, 경과 시간 한도를 설정하세요.

| 종료 코드 | 의미 |
| --- | --- |
| `0` | 완료 또는 부분 완료, 상태와 브랜치 확인 필요 |
| `2` | 실행 중단 또는 CLI 오류, 진단 확인 필요 |
| `4` | 안전 불변식 위반, 증거로 worktree 보존 |

`.refactor/runs/<id>/`에서 상태, 이벤트, 프로바이더 출력, 검증 근거와 리포트를 확인하세요. `last-run.json`은 최신 결과를 가리킵니다. 지원하는 기록은 현재 캐시 기본값과 독립적인 worktree 경로를 보관합니다. 지원하지 않는 과거 형식은 현재 CLI에서 읽거나 정리하지 않습니다.

## 로컬 개발 검사

검증 대상은 macOS Apple Silicon이며 Go 1.27과 Git이 필요합니다. 저장소 루트에서 실행합니다.

```bash
cd tool/go
go test -race ./...
go vet ./...
go build -o /private/tmp/refactor-me ./cmd/refactor-me
/private/tmp/refactor-me version --json
```

Go는 `gofmt`, `go vet`, `go test -race`, `go build`로 검증합니다. 기본 개발·검증에는 Node가 필요하지 않습니다. 과거 [계약 이관표](../docs/node-test-contracts.ko.md)는 Node 제거 당시의 기준선이며 이전 설치·리포트 호환 계약은 현재 릴리스에 적용되지 않습니다. [로컬 fixture 안내](fixtures/README.ko.md)에서 Go 예제와 선택적 JavaScript 예제를 만들 수 있습니다. **실제 모델의 판단 품질과 CLI의 Skill 로딩은 별도 프로바이더 실행으로 확인해야 합니다.**

Node 실행 차단까지 확인하려면 저장소 루트에서 다음 명령을 실행하세요. 이 검사는 프로젝트 도구의 Node 의존성을 확인하며 GitHub Actions 자체의 실행 환경을 제거하지는 않습니다.

```bash
bash tool/check-no-node.sh sh -c 'cd tool/go && go test -race -count=1 ./... && go vet ./...'
```
