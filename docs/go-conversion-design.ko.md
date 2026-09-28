# refactor-me Go 전환 설계 보고서

| 항목 | 결정 |
| --- | --- |
| 문서 기준 | 2026-09-28의 저장소 상태와 공식 문서 |
| 이번 작업 | 전환 준비와 비용을 판단하는 설계 보고서. Go 코드·릴리스는 만들지 않음 |
| 첫 배포 | 공개 Beta, macOS Apple Silicon (`darwin/arm64`) |
| 설치 | 릴리스 압축파일의 실행 파일을 대상 Git 저장소의 `.refactor`에 설치 |
| 전환 | 기존 Node 구현과 병행 검증한 후 기본 명령을 교체 |

## 결론부터

Go 전환으로 확실히 얻을 수 있는 것은 **refactor-me 자체를 실행하기 위한 Node.js 24 요구를 제거할 수 있다는 점**이다. 실행 파일 하나로 컨트롤러를 배포하고, 타입과 오류 경계를 Go 코드에서 명시할 수도 있다. 다만 Git, Codex/Claude CLI, 대상 프로젝트의 빌드 도구는 계속 필요하다. provider CLI 자체의 설치 요건까지 Go 전환으로 없어지는 것은 아니다.

가장 큰 비용은 `.mjs` 문법을 Go로 바꾸는 작업이 아니다. 현재 컨트롤러가 보장하는 **원본 checkout 격리, provider 권한과 실패 분류, timeout 뒤 자식 프로세스 정리, 상태 파일의 내구성, 과거 보고서 읽기, 안전한 재설치**를 새 구현에서 입증해야 한다. 이 계약을 통과하지 못하면 바이너리의 크기나 시작 속도가 좋아져도 전환하지 않는 편이 맞다. 내부 연산이 전체 실행 시간의 병목인지도 아직 측정되지 않았다. [기존 선택지 조사](tech-stack-migration-research.ko.md)

## 1. 현재 기준선과 유지할 계약

`refactor-me`는 `run`(기본값), `doctor`, `report`, `clean`, `version`, `help`를 제공한다. `--target`, `--provider`, `--fallback`, `--json`, `--lang`, `--no-live-probe`가 주요 옵션이다. `help`와 `version`은 Git 저장소 밖에서도 성공해야 한다. 종료 코드는 `0`(완료 또는 부분 완료), `2`(실행 전 중단·CLI 오류), `4`(안전 불변식 위반)다. 부분 완료의 세부 상태는 종료 코드만으로 구별할 수 없으므로 보고서를 봐야 한다. 근거: [CLI entrypoint](../tool/bin/refactor-me.mjs), [사용·종료 코드 설명](../tool/README.md#usage-and-exit-codes), [CLI 테스트](../tool/test/cli.test.mjs).

| 경계 | 현재 동작 | Go 전환 시 보존할 결과 |
| --- | --- | --- |
| 설치 | `install.mjs`가 소스를 `.refactor/lib`에 복사하고 `.refactor/bin/refactor-me` 셸 shim 생성 | 대상 저장소별 실행 명령, 기존 `config.json`·`runs/`·`last-run.json` 보존, 알 수 없는 사용자 파일 덮어쓰기 거부 |
| 설정 | `config.json`의 기존 필드를 기본값과 병합; `commands.json`의 locked 검증 명령 사용 | 기존 설정과 잠긴 검증 명령 읽기. 알 수 없는 값이나 손상된 파일의 오류 의미를 별도 fixture로 기록 |
| 영속 기록 | `state.json`, phase JSON, `report.json`, `report.md`, patch를 run 디렉터리에 기록 | 과거 `report.json`을 읽고 `report --lang en|ko`를 출력. `report --json`은 저장된 JSON 원문을 변경 없이 출력 |
| Git 격리 | 원본 HEAD·index·작업 파일을 보존하고 별도 detached worktree에서 작업; 승인된 결과를 로컬 ref에 compare-and-swap으로 발행 | 원본 보호와 충돌 시 안전 정지, 미완료·안전 정지 worktree 보존 |
| 실행 제어 | provider·검증 명령을 셸 없이 argv로 실행; timeout 뒤 프로세스 그룹에 SIGTERM, 10초 뒤 SIGKILL | 실패·timeout·실행 불가를 성공과 구분하고 남은 자식 프로세스를 정리 |

근거: [설치기](../tool/install.mjs), [설정](../tool/src/config.mjs), [상태·잠금](../tool/src/state.mjs), [Git 작업](../tool/src/git.mjs), [provider](../tool/src/provider.mjs), [검증 실행기](../tool/src/validate.mjs), [보고서](../tool/src/report.mjs).

**호환성의 한계.** 이 설계는 *과거 기록 읽기*와 주요 명령·종료 코드의 의미를 목표로 한다. 모든 내부 phase JSON 필드를 영원히 동일하게 쓰겠다는 약속은 아니다. 새 run 형식이 바뀌면 버전을 명시하고, 과거 기록은 변환하거나 덮어쓰지 않고 읽기 어댑터로 처리한다. 현재 provider 세션은 재개하지 않는 설계이므로, 중단된 Node run을 Go로 이어 실행하는 기능도 범위 밖이다. [provider 설계](../tool/src/provider.mjs)

## 2. 준비할 것

### 계약과 테스트 자료

1. **기준선 보관:** 현재 Node 24에서 `node --test tool/test/*.test.mjs`, `help`, `version --json` 결과와 CI 조건을 기록한다. 앞선 조사에서 257개 테스트가 통과했지만, Go 구현의 통과를 뜻하지 않는다. 현재 CI는 macOS에서 Node 24만 실행한다. [CI](../.github/workflows/verify.yml)
2. **비밀 없는 fixture:** 구버전 `config.json`, `commands.json`, `last-run.json`, `report.json`, 정상·손상 상태를 테스트 전용 임시 Git 저장소에 복제한다. 실제 run 디렉터리에는 prompt, provider 출력, 경로 또는 민감 정보가 있을 수 있으므로 그대로 테스트 자산에 넣지 않는다.
3. **차이 비교 항목:** CLI stdout/stderr와 exit code, 설정 기본값 병합, `report --json`의 바이트 일치, 영문·국문 보고서의 의미, Git 변경 전후의 HEAD·index·작업 파일, timeout·quota·auth·schema 오류 분류를 각각 비교한다.
4. **실행 환경:** Go toolchain 버전을 저장소와 CI에 고정하고 `go.mod`/`go.sum`을 관리한다. 우선 표준 라이브러리로 설계하되 외부 패키지가 꼭 필요하면 목적·라이선스·업데이트 책임을 검토한 뒤 별도 승인한다. `darwin/arm64`의 실제 실행 테스트를 릴리스 필수 조건으로 둔다. [Go module 문서](https://go.dev/doc/tutorial/create-module)
5. **공개 배포 준비:** archive 무결성 검증, 빌드 출처, 버전 일치, Apple Developer ID 서명 및 notarization 가능 여부를 릴리스 게이트로 문서화한다. Apple은 notarization 제출물에 유효한 Developer ID 서명, hardened runtime, timestamp 등을 요구한다. 자격 증명과 외부 제출은 이번 작업에서 확인하거나 실행하지 않았다. [Apple 공식 안내](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution)

현재 Go 개발 빌드는 `dev`로 표시하며 다음 공개 릴리스는 `0.9.20-beta.1`부터 시작한다. 공개 업로드는 이 전환 작업에 포함하지 않는다.

## 3. 가장 많이 바뀌는 부분

### 설치와 업데이트 — 배포 계약 자체가 바뀐다

현재 설치는 Node 소스를 복사하고 셸 shim을 만든다. Go 버전은 압축파일에서 꺼낸 실행 파일이 저장소 루트를 확인한 다음 **자기 자신을** `.refactor/bin/refactor-me`로 설치하는 형태를 권장한다. 사용자의 저장소에는 Go toolchain이 필요하지 않다. 기존 `.refactor/lib`, `config.json`, `runs/`, `last-run.json`, 사용자 정의 명령은 교체 대상이 아니다. 이미 있는 `refactor-me`가 도구가 만든 shim인지 식별할 수 없으면 중단한다. 설치 실패가 기존 실행 명령을 망가뜨리지 않도록 임시 경로에 복사·검증한 뒤 교체하는 절차와 복구 경로를 설계해야 한다. 실제 파일 교체와 이전 버전 보존 방법은 시제품의 설치 테스트로 검증한다. [현 설치·재설치 테스트](../tool/test/cli.test.mjs)

### 하위 프로세스 — `CommandContext`만으로 충분하지 않다

Node 구현은 provider와 검증 명령을 별도 프로세스 그룹으로 시작하고 그룹 전체에 SIGTERM을 보낸 뒤 10초 후 SIGKILL을 보낸다. Go의 `os/exec`는 셸을 호출하지 않는다는 점에서 argv 계약과 맞지만, `CommandContext`의 기본 취소는 프로세스 자체의 `Kill`이다. **자식·손자 프로세스까지 정리하는 현재 의미는 별도 프로세스 그룹 처리와 테스트가 필요하다.** 표준 출력 JSONL 파싱, stderr 분리, stdin 전달, timeout과 spawn 실패의 구분, provider 환경 변수 allowlist도 함께 이식해야 한다. 특히 환경 값 자체를 보고서·로그·fixture에 넣으면 안 된다. [Go `os/exec`](https://pkg.go.dev/os/exec), [현 provider 실행](../tool/src/provider.mjs), [현 검증 실행](../tool/src/validate.mjs)

### Git, 상태와 정책 — 코드량보다 의미의 검증 비용이 크다

Git은 라이브러리의 단순 치환 대상이 아니다. 현재 `git`의 argv·작업 디렉터리·출력·오류를 판정하고, 분리 worktree의 tree가 검토한 tree와 일치할 때만 ref를 compare-and-swap으로 갱신한다. Go 구현은 같은 실패 경계와 rollback 범위를 입증해야 한다. 동시에 `.refactor/lock.json`의 배타적 생성·소유자 확인과 `state.json`의 임시 파일 기록→`fsync`→rename을 보존해야 한다. 기존 상태의 `schema`와 `toolVersion`은 다른 의미이므로 합치지 않는다. [Git 계약](../tool/src/git.mjs), [상태 저장](../tool/src/state.mjs), [worktree 게이트 테스트](../tool/test/worktree-gate.test.mjs)

`prompts.mjs`, `schemas.mjs`, `gate.mjs`, `loop.mjs`에는 모델 출력 계약과 단계별 중단 정책이 들어 있다. Go로 옮길 때 정적 타입만으로 외부 JSON 입력을 신뢰해서는 안 된다. 응답 schema 검증, repair 횟수, 쓰기 가능 phase의 fail-closed 판정, provider 전환 조건, 검증 baseline의 `GREEN/RED/TIMEOUT/UNRUNNABLE/OPAQUE` 구분을 fixture로 비교한다. [provider 테스트](../tool/test/provider.test.mjs), [workflow 테스트](../tool/test/workflow.test.mjs), [검증 테스트](../tool/test/validate.test.mjs)

## 4. 기대 이득과 지불할 비용

| 항목 | 얻는 것 | 비용·단점 | 확인 방법 |
| --- | --- | --- | --- |
| 사용자 설치 | refactor-me 자체의 Node 24 선행 설치 제거; 대상 저장소에 Go 설치 불필요 | 압축파일 선택·검증, 바이너리 교체·복구 절차가 새 운영 책임 | 깨끗한 macOS ARM 환경의 설치→재설치→제거 실습 |
| 코드 안정성 | Go 타입·명시적 오류 반환으로 내부 경계를 드러내기 쉬움 | JavaScript의 동적 JSON과 provider 응답을 모델링·검증하는 코드 및 테스트 증가 | 구버전 fixture와 malformed 입력을 동일하게 분류 |
| 실행 속도·자원 | 내부 CLI 시작·파싱 비용을 줄일 가능성 | 실제 run은 모델 응답·Git·대상 프로젝트 검증을 기다릴 수 있어 총시간 이득 미확인 | 외부 대기와 controller 내부 시간을 분리해 동일 작업 비교 |
| 배포·보안 | 플랫폼별 바이너리와 릴리스 산출물의 버전 추적 가능 | CI 빌드, 서명, notarization, 압축파일 검증, 공급망 관리 추가 | 배포물 검증과 새 macOS 호스트 실행 |
| 유지보수 | 단일 Go runtime과 표준 도구로 개발 가능 | Node 테스트·문서·설치 경로를 한동안 병행; 버그 수정이 두 구현에 중복될 수 있음 | 병행 기간과 제거 기준을 정하고 차이 목록 관리 |

비용을 시간이나 금액으로 단정하지 않는다. 아직 Go 시제품과 대표 run 프로파일이 없다. 우선 높은 위험의 프로세스 제어·Git 격리·설치 안전성에 검증 비용을 배정하고, 전체 run 성능은 **모델 호출 시간 / 프로젝트 검증 시간 / controller 내부 시간**으로 나눠 측정해야 한다. 한 번의 wall time만으로 언어의 성능 차이를 주장하지 않는다.

## 5. 사용자가 받게 될 최종 형태

다음은 **권장하는 산출물 설계**이며 현재 존재하는 파일·명령이 아니다. 첫 공개 릴리스는 `darwin/arm64`만 대상으로 한다. Intel, Linux, Windows 실행 파일은 같은 코드로 컴파일될 가능성과 별개로 각각 설치·프로세스·Git 동작을 시험한 뒤 지원 여부를 결정한다. Go 소스는 기존 Node 구현과 병행할 수 있도록 `tool/go/` 안에 별도 module로 두고, `cmd/refactor-me`를 실행 진입점으로 삼는 구성을 권장한다. 새 `internal/` package는 실제 계약 경계가 확인될 때만 나눈다.

```text
GitHub release vX.Y.Z
├── refactor-me_vX.Y.Z_darwin_arm64.zip
│   ├── refactor-me        # Go 실행 파일; install/uninstall과 CLI 명령 제공
│   ├── LICENSE
│   └── INSTALL.md
└── SHA256SUMS             # 배포 파일 무결성 확인용

target-repo/.refactor/
├── bin/refactor-me        # 설치된 Go 실행 파일
├── bin/refactor-me-node   # 병행 검증·복구 기간에만 보존할 기존 shim
├── lib/                   # 기존 Node 설치물; 전환 완료 전 보존
├── config.json            # 기존 사용자 설정 유지
├── commands.json          # 있으면 유지
├── last-run.json          # 기존 최신 run 포인터 유지
└── runs/                  # 기존 및 새 실행 기록 유지
```

예상 사용 흐름도 제안이다. 압축파일의 무결성과 서명 상태를 확인한 후 실행한다. `install`은 지정한 경로가 Git 저장소인지 확인하고, 소유하지 않은 기존 파일을 만나면 교체 전에 멈춰야 한다.

`SHA256SUMS`는 다운로드 손상 확인에 쓰지만, 같은 게시자가 만들었다는 증거를 단독으로 제공하지는 않는다. 공개 배포 전 서명·notarization 결과와 배포 경로를 함께 검증한다. 서명할 수 없는 개발용 시제품과 사용자에게 제공할 릴리스는 명확히 구분한다.

```bash
./refactor-me install /path/to/target-repo
cd /path/to/target-repo
./.refactor/bin/refactor-me version --json
./.refactor/bin/refactor-me doctor --no-live-probe
./.refactor/bin/refactor-me report --lang ko
```

`doctor --no-live-probe`는 provider 모델 호출을 건너뛰지만 진단 기록을 쓸 수 있으므로 읽기 전용 명령으로 소개하지 않는다. 실제 `run`은 현재와 같이 별도 worktree에서 일하고 승인된 결과만 로컬 branch에 둔다. 기존 `report --json`은 `last-run.json`이 가리키는 **저장된 `report.json` 원문**을 그대로 내보낸다. Go가 만드는 새 기록은 형식을 버전 관리하되 과거 JSON을 자동 재작성하지 않는다. `version --json`의 Node 전용 `node` 필드는 Go에서 사실처럼 채우지 않는다. 이를 사용하는 소비자가 있는지 조사한 뒤 새 runtime 표시와 호환성 변경을 명시한다.

업데이트는 같은 `install` 절차로 수행하며 설치된 실행 파일을 교체해도 과거 설정과 실행 증거를 보존한다. 제거는 Go 실행 파일 등 **도구 소유가 확인된 파일**에 한정하고, 설정·실행 기록·결과 branch·증거용 worktree는 지우지 않는 방향으로 설계한다. 전환을 되돌릴 때는 보존된 Node 설치물과 shim을 수동으로 복원하는 절차를 문서화한다. 이 복구 경로는 릴리스 전에 실제 임시 저장소에서 시험해야 한다.

## 6. 구현 순서와 확대 조건

| 단계 | 산출·검증 | 다음 단계로 갈 조건 |
| --- | --- | --- |
| A. 계약 고정 | Node CLI·설치·report·실패 경로의 비밀 없는 fixture와 기준선 | 외부 계약과 허용할 변경을 명시 |
| B. 작은 Go 시제품 | `help`, `version`, 설정 병합, 과거 `report --json`/영·국문 렌더링 | 원문 JSON 보존, 구버전 fixture와 exit code 통과 |
| C. 설치 시제품 | `darwin/arm64` 빌드와 archive, 저장소별 설치·재설치·복구 | 기존 사용자 파일 보존, 소유 불명 파일에서 중단, 실패 중 기존 명령 유지 |
| D. 실행 컨트롤러 | provider, schema, Git, worktree, gate, validation, report 이식 | 프로세스 그룹 timeout, 원본 보호, 실패 분류를 fixture로 입증 |
| E. 병행 검증·전환 | 같은 입력·fixture로 Node/Go 비교, 공개 배포 준비 | 주요 차이가 설명·승인되고 macOS ARM 실기기 및 서명·배포 게이트 통과 |

첫 시제품에서 provider 호출이나 Git write를 곧바로 구현하지 않는다. 차이가 발견되면 기존 Node 출력을 무조건 정답으로 삼지 않고, 문서화된 계약과 안전 정책에 비춰 판정한다. 특히 기존 `git commit` 경로에는 `--no-verify`가 쓰인다. 이를 Go에 그대로 복제할지는 별도 정책 결정이 필요하며, 이 조사에서는 실행하거나 승인하지 않는다. [현 Git 구현](../tool/src/git.mjs)

**전환 중단 조건:** 원본 checkout이 변경되거나, 구버전 보고서가 잘못 읽히거나, subprocess 실패·timeout이 성공으로 처리되거나, 설치 실패가 기존 명령을 손상하면 기본 명령 교체를 중단한다. live provider 확인은 로컬 fixture·테스트와 별도 결과로 보고한다. 공개 릴리스 업로드, 서명 및 notarization 제출, 기존 설치 교체는 모두 이번 보고서의 실행 범위 밖이다.

## 근거와 검증 범위

- **저장소에서 확인:** 위 링크의 현행 코드·테스트·CI, 기존 [기술 스택 조사](tech-stack-migration-research.ko.md). 현재 `git status`에는 이 문서와 무관한 Skill·lockfile 수정이 있으므로 보존한다.
- **공식 자료에서 확인:** Go의 [빌드](https://go.dev/doc/tutorial/compile-install), [`os/exec`](https://pkg.go.dev/os/exec), Apple의 [notarization 요건](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution).
- **아직 검증하지 않음:** Go 시제품, archive·서명·notarization, macOS 새 호스트 설치, provider live run, 실제 시간·메모리·파일 크기. 이 항목들은 기대 효과나 완료 사실로 쓰지 않았다.
