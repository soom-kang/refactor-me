# refactor-me 상세 문서

[프로젝트](../docs/README.ko.md) · [English](README.md) · [실행 가이드](TUTORIAL.ko.md) · [Workflow](WORKFLOW.ko.md) · [변경 이력](CHANGELOG.md)

실행 설정과 검사 결과를 해석할 때 참고하세요. 설치와 결과 확인 절차는 [실행 가이드](TUTORIAL.ko.md)에 있습니다.

## 후보 선택

Audit는 다음 네 종류의 후보를 찾습니다.

| 분류 | 후보 |
| --- | --- |
| `DEAD_CODE` | 도달 가능한 진입점에서 더는 참조하지 않는 코드 |
| `COMPATIBILITY_REMOVAL` | 사용하지 않는 호환성 코드나 마이그레이션 코드 |
| `DEDUPLICATION` | 세 곳 이상에서 반복되는 로직 |
| `LARGE_COMPONENT_SPLIT` | 기존 경계를 따라 나눌 수 있는 500줄 초과 파일 |

동작 보존 범위에는 반환값, 부수 효과, 순서, 오류, 화면 출력과 저장 데이터 형태가 포함됩니다. 기존 진입점과 공개 인터페이스에서 관찰할 수 있는 동작을 기준으로 판단합니다. 버그 수정과 추측에 따른 개선은 후보에서 제외합니다.

`--target <dir>`은 여러 번 지정할 수 있으며 명령을 실행한 디렉터리를 기준으로 해석합니다. 후보 탐색 범위를 제한합니다. 호출부와 도달성은 저장소 전체에서 확인하고, 후보 검증은 변경된 영역을 선택하고 루트 영역이 있으면 함께 실행합니다. 후보는 대상 폴더와 관련이 있어야 하며 변경 결과에도 대상 파일이 포함되어야 합니다. 잘못된 경로나 추적 중인 소스 파일이 없는 대상은 실행을 막습니다.

## 실행과 안전 검사

Audit, deep check, 필요한 경우 characterization, preflight, 실행, 검증, 독립 리뷰 순서로 진행합니다. 변경을 커밋하면 다시 조사합니다.

원본 저장소 밖에 detached worktree를 만듭니다. 의존성 디렉터리와 로컬 환경 파일 등 gitignore 대상 빌드 입력을 worktree에 복사할 수 있습니다. Worktree와 실행 기록에도 원본 저장소와 같은 접근 통제를 적용하세요.

구현 후 컨트롤러는 고정된 작업 명세를 기준으로 변경을 검사합니다. 금지 경로, 허용 파일, 변경 규모, 분류별 크기 규칙, 테스트 무결성과 이전 tree hash를 확인합니다. 원본 checkout과 승인된 worktree 상태가 예상치 않게 바뀌었는지도 검사합니다. 거절한 후보는 실행용 worktree 안에서 되돌립니다.

로컬 커밋과 결과 브랜치를 만들며 merge, push, 배포는 수행하지 않습니다. 애플리케이션 의존성을 별도 단계로 설치하지 않지만 기존 검증 명령과 빌드 도구가 네트워크를 사용하거나 캐시를 채울 수 있습니다.

`sharpen-cold-review`는 별도 프로바이더 세션에서 실행합니다. 두 프로바이더가 사용 가능하고 교차 리뷰 설정이 켜져 있으면 구현하지 않은 프로바이더를 우선합니다. 쿼터나 인증 문제로 같은 단계 안에서 프로바이더를 전환할 수 있습니다. 새 세션에는 해당 단계의 입력을 전달하며, 작업 명세가 필요한 단계라면 고정된 명세도 포함합니다. `handoff.md`는 전환 시점의 기록이고 최종 결과는 리포트에서 확인합니다.

## Skill 설치 상태

`soom-kang/sharpen-me`의 Skill 8개를 대상 저장소에 설치하고 파일과 에이전트 링크를 커밋하세요. 각 Skill의 역할은 [프로젝트 README](../docs/README.ko.md#단계별-skill)에 있습니다.

Doctor는 필수 Skill의 디스크 경로를 확인합니다. Claude는 `--setting-sources project` 조건에서 `.claude/skills`를 사용합니다. Codex는 `.agents/skills`와 지원하는 홈 경로를 확인합니다. 저장소에 설치한 Skill은 기준 checkout에도 있어야 합니다.

실제 프로바이더 검사에서는 세션이 볼 수 있는 Skill도 묻습니다. 디스크에 전부 설치되어 있으면 일부만 보고했다는 이유로 실패시키지 않습니다. 세션이 하나도 볼 수 없다고 보고하면 실패합니다. `--no-live-probe`는 이 확인을 생략합니다. Doctor는 리팩터링하지 않지만 진단 기록을 쓰고 기본 설정에서는 프로바이더를 호출합니다.

## 설정

설치기는 `.refactor/config.json`이 없을 때만 만듭니다. 재설치해도 기존 파일은 유지합니다. 실행 기본값은 `src/config.mjs`와 `src/loop.mjs`, 설치용 전체 설정은 [config.default.json](config.default.json)에 있습니다.

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

기본 추론 수준은 audit, deep check, 실행, 리뷰에서 `high`, preflight와 characterization에서 `medium`, doctor와 handoff에서 `low`입니다. 재조사는 `medium`을 요청하며 프로바이더 설정으로 바꿀 수 있습니다.

`schema_version`과 `verification`은 설정 형식에 남아 있습니다. 현재 루프는 검증 명령을 고를 때 `verification.locked`를 사용하지 않습니다. 직접 지정하려면 아래의 `.refactor/commands.json`을 사용하세요.

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

Gradle은 JVM 플러그인이 적용되어 있어야 탐색합니다. 실행 가능한 Gradle·Maven wrapper가 있으면 PATH 실행 파일보다 우선합니다. 상위 JVM 빌드가 하위 모듈을 담당합니다. 브라우저, E2E, 외부 서비스가 필요한 것으로 감지한 검사는 제외하고 리포트에 기록합니다.

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

ID는 서로 달라야 합니다. `area`와 `cwd`는 저장소 상대 경로, `argv`는 인자 배열, `tier`는 `T1`, `T2`, `T3` 중 하나로 지정하세요. Locked 파일이 있으면 자동 탐색을 대체합니다. 컨트롤러가 worktree에서 이 명령을 실행하므로 먼저 내용을 검토하세요.

기준선에서는 통과, 오류를 해석할 수 있는 실패, 실행 불가, 유효한 signature가 없는 실패를 구분합니다. 기준선에서 실패한 명령은 이후 실패와 비교하며 통과한 검사로 집계하지 않습니다. 새 오류가 생기거나 통과했던 명령이 실패하면 후보를 거절합니다. 통과 신호가 없는 기준선에서는 실행을 중단합니다. 기준선은 발견한 영역의 선택된 명령을 실행합니다. 후보 수정 후에는 경로별로 가장 깊은 변경 영역을 선택하고, 루트 영역이 있으면 함께 검증합니다. 기준선의 `TIMEOUT`, `UNRUNNABLE`, `OPAQUE` 명령은 제외하며 첫 실패에서 검증을 멈춥니다. 이 결과만으로 저장소의 모든 영역이 검증을 통과했다고 판단할 수는 없습니다.

## 리포트와 언어

`run`과 `report`에서 `--lang en|ko`를 지원하며 기본값은 `en`입니다. 종료 요약, `report.md`와 handoff의 고정 안내에 적용합니다. 진행 로그와 doctor 출력은 영어를 유지합니다. 모델 설명, 오류 원문, 경로와 커밋 제목은 번역하지 않습니다.

실행마다 `report.md`와 `report.json`을 하나씩 저장합니다. `report --lang ko`는 최신 JSON을 읽어 한국어로 출력하며 저장된 파일을 갱신하지 않습니다. 버전이나 사용량 필드가 없는 과거 JSON도 읽습니다. JSON이 없거나 잘못된 형식이면 오류를 내고 기존 Markdown은 보존합니다. `--json` 결과는 언어와 무관합니다.

코드 비교는 원본 저장소의 `state.baseOid`와 `state.publishedOid`를 기준으로 합니다. Worktree를 정리한 뒤에도 characterization 커밋을 포함한 최종 반영 내용을 비교합니다. 파일 통계와 주변 문맥 3줄을 포함한 diff 미리보기를 표시하며, 미리보기는 완전한 줄 단위로 최대 200줄 또는 UTF-8 32 KiB까지 담습니다. `changes.patch`에는 전체 텍스트 patch를 저장합니다. 바이너리 본문은 생략하고 바이너리·파일 모드 변경은 메타데이터로 남깁니다. 외부 diff와 textconv는 사용하지 않습니다.

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

반영한 커밋이 없으면 patch 없이 `NO_CHANGES`를 기록합니다. 반영한 커밋이 있어도 두 tree가 같으면 빈 patch를 저장합니다. Git 객체가 없거나 patch 저장에 실패하면 `UNAVAILABLE`과 사유를 기록하고 실행 상태와 종료 코드는 유지합니다. 이 필드가 없는 과거 JSON은 비교 미저장 안내를 표시합니다. 리포트를 조회할 때는 비교를 다시 수집하지 않습니다. `accepted.patch`는 리뷰 전 스냅샷이므로 거절된 후보에도 남을 수 있습니다. 커밋된 변경은 최종 비교에서 확인하세요.

사용량에는 실패한 호출과 스키마 복구 프로세스도 포함합니다. 비용을 보고하지 않으면 미보고로 표시하고, 일부 비용만 있으면 최소 금액으로 표시합니다. 루프는 전체 금액 예산을 강제하지 않습니다. Claude 예산 옵션은 프로바이더 설정이며 실행 전체의 한도가 아닙니다. 사이클, 커밋과 경과 시간 한도로 실행을 제한하세요.

| 종료 코드 | 의미 |
| --- | --- |
| `0` | 완료 또는 부분 완료, 상태와 브랜치 확인 필요 |
| `2` | 실행 중단 또는 CLI 오류, 진단 확인 필요 |
| `4` | 안전 불변식 위반, 증거로 worktree 보존 |

`.refactor/runs/<id>/`에는 상태, 이벤트, 프로바이더 출력, 검증 근거와 리포트를 저장합니다. `last-run.json`이 최신 결과를 가리킵니다. 실행 기록에 worktree 경로가 있으므로 기본 캐시 경로가 바뀌어도 과거 기록을 사용할 수 있습니다.

## 로컬 개발 검사

저장소 루트에서 실행합니다.

```bash
node --test tool/test/*.test.mjs
node --check tool/bin/refactor-me.mjs
node tool/bin/refactor-me.mjs version --json
```

도구에는 별도 lint, typecheck, build 명령이 없습니다. 테스트는 Node.js 기본 모듈과 임시 저장소를 사용합니다. 프로바이더의 판단 품질과 실제 CLI의 Skill 로딩은 별도 실행으로 확인해야 하며 단위 테스트만으로 입증되지 않습니다.
