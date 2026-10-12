# QA remediation — 2026-10-12

[티켓의 Resolution](2026-10-12-tickets.md)에서 수정 결과를 확인합니다. `RM-QA-001`을 먼저 검증한 뒤 독립된 P2 범위를 병렬로 수정하고 P3 표시 문제까지 처리했습니다.

## 현재 상태

**12개 티켓을 로컬 구현과 검증 기준으로 해결했습니다.** 수정은 현재 working tree에 있습니다. 기준 HEAD는 `0229e141d59cf6b248c422ac5b4dbb482fd37813`이며 commit을 만들지 않았습니다. 공개 Beta.6 executable, formula, tag와 배포 상태는 갱신하지 않았습니다. 설치된 Beta.6에 이 수정이 반영됐다는 뜻은 아닙니다.

| 범위 | 구현 결과 |
| --- | --- |
| RM-QA-001 / 002 | 잘못된 locked 파일은 preparation 전에 오류, 누락된 area/cwd는 `.`, 0-check ladder는 실패. 검증 cwd의 lexical/실제 경계를 검사 |
| RM-QA-003 / 005 | 안전하지 않은 링크가 있으면 빌드 입력 전체 생략. Source HEAD/fingerprint를 commit 전과 publication CAS 직전에 재검사 |
| RM-QA-004 / 012 | 알려진 설정의 type·정수·범위·precision 검증. Audit와 packet에서 `L3_CRITICAL`을 항상 제외 |
| RM-QA-006 / 007 / 008 | 호출별 transcript/exit/usage 보존. 취소는 `VALIDATION_CANCELLED` 및 ABORTED로 구분하고 미완료 변경 보존. Claude nonzero/missing exit 거부 |
| RM-QA-009 / 010 / 011 | Human report의 제어 문자 제거와 UTF-8 경계 절단. Public packaging의 exact annotated tag 및 HEAD 일치 검사 |

기존 config schema 2와 report schema 3, raw JSON, `changes.patch`, 사용자가 편집한 checklist를 보존합니다. 새 production dependency나 test infrastructure는 추가하지 않았고 기존 test 파일만 보완했습니다.

## 합의와 호환성

사용자가 두 제품 계약을 확정했습니다.

1. `L3_CRITICAL`은 custom `allowed_risks`에 들어 있어도 차단합니다.
2. 외부 링크가 포함된 hydration 입력은 전체를 생략합니다. 일부 dependency만 복사하지 않습니다.

Hydration은 복사 전후 링크를 검사합니다. Absolute link는 복사하면 원본 target을 계속 참조하므로 생략하며, 외부 또는 dangling link도 생략합니다. 복사된 worktree 안에서 해석되는 relative link는 지원합니다. 기존 destination은 바꾸지 않고 새로 복사하다 실패한 입력만 `os.Root` 경계 안에서 정리합니다. 진단에는 상대 입력 경로와 고정 이유만 표시합니다. 생략된 dependency는 사용자가 worktree에 맞게 준비해야 할 수 있습니다.

알려진 설정값을 잘못된 type으로 지정하면 defaults로 바꾸지 않고 정확한 key와 기대값을 안내합니다. Unknown extension key와 생략된 defaults는 유지합니다. 기존 `0`의 의미도 유지했습니다.

| 설정 | `0`의 기존 의미 |
| --- | --- |
| `max_consecutive_failures` | 다음 audit 경계에서 즉시 종료 |
| `max_attempts_per_fingerprint`, `max_files_per_candidate`, `max_audit_candidates` | 해당 후보 허용/선택 한도가 0 |
| `max_changed_lines` | 기본 600줄 사용 |
| `agents.<provider>.timeout_sec` | 기본 1800초 사용 |
| `cooldown_minutes` | cooldown 지연 없음 |
| `agents.claude.max_budget_usd` | 예산 옵션을 전달하지 않음 |

`max_cycles`, `max_commits`, `empty_audits_to_stop`, `max_wall_clock_min`은 기존대로 양수입니다. 정수 count/time 필드의 소수, 음수, duration/native-int overflow와 JSON number precision loss를 거부하며 Claude USD budget은 유한한 0 이상의 소수를 허용합니다.

Transcript는 첫 `primary`/`repair` 경로를 유지하고 retry를 `primary-retry-N`으로 구별합니다. 기존 bundle과 충돌하면 suffix를 붙여 새 경로를 예약합니다. `.result.json`에 각 호출의 exit/usage/status, repair에는 `repairOf`를 남깁니다. Empty stderr도 별도 파일로 저장합니다. Codex `phase-last.json`은 직렬 실행의 최신 조회 파일이며 각 bundle의 `.last.json`이 원본 응답을 보존합니다.

## 검증

관련 Go package 전체의 test 결과, concurrency 범위의 race, vet, build, lint와 packaging 검증이 통과했습니다. 최초 전체 실행의 실패와 후속 재검사를 아래에 구분합니다. 변하지 않은 passing package는 반복하지 않았습니다.

| 검사 | 결과 |
| --- | --- |
| Node guard / gofmt / Go tests (6 internal packages, 누적 결과) | PASS |
| `go vet ./...` / development build | PASS |
| controller / engine / workspace race (누적 결과) | PASS |
| repository golangci-lint | PASS — 0 issues |
| Python packaging tests 4개 / `bash -n` | PASS |
| 문서 링크 / anchors / final diff / credential signature | PASS |

Repository root에서 실행한 기본 명령입니다. 첫 전체 test invocation은 아래에 기록한 fixture 문제로 실패했으며 vet/build는 그 호출에서 실행되지 않았습니다.

```sh
GOCACHE=/private/tmp/refactor-me-fix-go-cache \
  bash tool/check-no-node.sh bash -e -c '
    cd tool/go
    test -z "$(gofmt -l .)"
    go test -count=1 ./...
    go vet ./...
    go build -o /private/tmp/refactor-me-qa-fixed-20261012 ./cmd/refactor-me
  '
```

`tool/go`에서 concurrency와 lint를 검사합니다.

```sh
GOCACHE=/private/tmp/refactor-me-fix-go-cache \
  go test -race -count=1 ./internal/controller ./internal/engine ./internal/workspace
GOCACHE=/private/tmp/refactor-me-fix-go-cache \
  GOLANGCI_LINT_CACHE=/private/tmp/refactor-me-fix-lint-cache \
  golangci-lint run --config ../../.golangci.yml
```

Release 변경은 기존 `python3 -m unittest discover -s tool/release -p 'test_*.py'` 4개와 `bash -n tool/release/build-macos-arm64.sh`로 검증했습니다. Missing/branch-only/lightweight/wrong-HEAD tag는 artifact 작성 전 거부하고 annotated HEAD 및 candidate mode는 기존 경로를 유지합니다. Binary test의 compiler는 stub이며 실제 release binary를 만든 검증은 아닙니다.

초기 전체 `go test -count=1 ./...`에서 controller(85.639초), catalog(1.212초), fixture(11.692초), surface(4.826초)는 통과했습니다. Engine의 `TestTaxonomy`는 정상 process를 모사한 fixture에 exit code가 없어서 실패했고, workspace의 `unrunnable-skip`은 0-check 성공을 기대해 새 계약과 충돌했습니다. 두 fixture를 실제 성공 exit `0` 및 명시적 0-check 실패로 수정했습니다. Engine의 child-after-leader-exit는 병렬 test/race 실행에서 1초 timeout 후 PID 파일이 없었습니다. 단독 targeted 실행에서는 원래 timeout과 descendant assertion 그대로 통과했습니다. `[Inference]` 실행 부하에 민감한 fixture일 가능성이 있으나 원인을 독립적으로 확정하지는 않았습니다.

변경된 engine/workspace만 다음 명령으로 재검사했습니다. Engine 16.256초, workspace 9.330초 모두 PASS였고, 이어진 vet/build도 통과했습니다. 다른 passing package의 source는 바뀌지 않아 결과를 재사용했습니다.

```sh
cd tool/go
GOCACHE=/private/tmp/refactor-me-fix-go-cache go test -count=1 ./internal/engine ./internal/workspace
GOCACHE=/private/tmp/refactor-me-fix-go-cache go vet ./...
GOCACHE=/private/tmp/refactor-me-fix-go-cache go build -o /private/tmp/refactor-me-qa-fixed-20261012 ./cmd/refactor-me
GOCACHE=/private/tmp/refactor-me-fix-go-cache go test -race -count=1 ./internal/engine ./internal/workspace
```

Race 최초 실행의 controller는 88.338초 PASS였습니다. 같은 fixture 문제를 고친 뒤 engine 18.281초, workspace 11.619초로 재검사를 통과했습니다. 원래 전체/race command가 처음부터 성공했다고 표현하지 않습니다.

개발 binary metadata는 `dev`, `commit:unknown`, Go 1.27.2/darwin/arm64입니다. 이 binary의 정상/Unicode/control report 입력을 EN/KO로 실행해 exit 0, valid UTF-8, 제어 문자 제거와 다중 행 보존을 확인했습니다. 모든 입력에서 `report --json`과 저장 JSON의 원본 bytes도 같았습니다.

초기 focused/controller package 검사에서 의도적으로 보존된 dirty fixture를 `WorktreeRemove`로 지우려는 cleanup 실패가 있었습니다. 양쪽 disposable repository의 `TempDir` cleanup에 맡겨 해결했습니다. 실제 rollback/commit assertion과 product의 unsafe removal 거부는 유지했습니다. Root의 첫 gofmt 명령은 working directory와 상대 경로가 맞지 않아 실행되지 않았으며 경로를 바로잡아 수행했습니다. 실패를 acceptance 통과로 기록하지 않았습니다.

## 검증 한계와 repository 상태

| NOT_RUN | 이유와 후속 조건 |
| --- | --- |
| `govulncheck ./...` | scanner 없음. 최신 vulnerability DB 판정은 미확인 |
| 실제 Codex/Claude 및 public CLI SIGINT | 최소 deterministic regressions와 context cancellation로 검증. 비용·인증이 있는 live fixture는 별도 범위 |
| 새 Homebrew 설치·upgrade / standalone release artifact | 공개 배포 변경을 요청하지 않았고 기존 설치/trust를 보존. Packaging tests와 실제 설치 증거는 구분 |
| 선택적 JS fixture | Runtime의 Node 요구 사항은 변경하지 않음. JS target actual execution은 별도 검사 |

검증 cwd와 hydration 검사도 trusted argv 전체의 OS sandbox는 아닙니다. 외부 process의 임의 side effect나 동시에 악의적으로 filesystem을 교체하는 공격까지 차단했다고 주장하지 않습니다. Source 검사는 기존 clean-source 전제의 HEAD/status fingerprint를 사용하며 filesystem lock은 아닙니다.

시작 상태는 이전 QA 문서 9개 수정과 QA 파일 2개 신규입니다. 해당 변경을 보존하고 티켓 해결에 필요한 runtime, 기존 tests, release gate와 관련 문서만 수정했습니다. Version, Skill catalog, config 파일, 기존 실행 기록과 credential은 변경하지 않았습니다. 외부 JIRA 등록, commit·push·배포는 수행하지 않았습니다.

최종 범위는 기존 tracked 파일 38개 수정과 QA 문서 3개 신규입니다. Runtime 12개 Go 파일과 관련 기존 tests, release guard 3개 파일 및 문서에 한정했습니다. `git diff --check`와 relative-link/anchor 검사, 중복 없는 ticket ID/Resolution 12개, credential signature와 생성한 예시의 수동 확인을 완료했습니다. 역사적 QA의 source line 링크는 기준 commit에 고정해 수정 후 line 이동과 구분했습니다. 검사 도구와 문서 교열을 제품 전체의 secret/security audit로 확대하지 않습니다.

현재 working tree의 [티켓 Resolution](2026-10-12-tickets.md)을 검토하면 구현과 acceptance 근거를 함께 확인할 수 있습니다.
