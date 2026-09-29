# refactor-me Go 전환 설계 보고서

> **2026-09-29 보관 기준:** 이 문서는 Homebrew 전환 전 조사·검증 기록입니다. 본문의 프로젝트별 설치, Node rollback, 과거 JSON 호환과 Skill 커밋 요건은 `0.10.0-beta.1`에 적용되지 않습니다. 현재 절차는 [Homebrew·전역 Skill 안내](README.ko.md), 명령·형식 계약은 [현재 참조 문서](../tool/README.ko.md)를 따르세요. 아래 테스트명과 실행 결과는 당시 근거이며 현재 테스트 목록이나 새 릴리스의 검증 결과가 아닙니다.

> **과거 전환 자료:** 본문은 2026-09-28 조사·설계와 당시 릴리스 준비 판단을 보존한 기록입니다. 현재 개발 절차는 [Go 개발 안내](../tool/README.ko.md#로컬-개발-검사), 테스트 이관 상태는 [계약 이관표](node-test-contracts.ko.md)를 따릅니다. 아래 Node 코드 링크는 삭제 전 고정 commit을 가리키며 현재 checkout의 실행 경로가 아닙니다.
| 항목 | 결정 |
| --- | --- |
| 문서 기준 | 2026-09-28의 저장소 상태와 공식 문서 |
| 문서 성격 | Go 전환 전에 작성한 판단 자료. 아래 구현 현황과 릴리스 결정은 이후 갱신 사항 |
| 첫 배포 | 공개 Beta, macOS Apple Silicon (`darwin/arm64`) |
| 설치 | 릴리스 압축파일의 실행 파일을 대상 Git 저장소의 `.refactor`에 설치 |
| 구현 현황 | Go CLI와 저장소별 설치기가 `tool/go`에 구현됨. 첫 Go Beta 릴리스 검증 중 |

**2026-09-28 릴리스 결정:** 첫 Go Beta `0.9.20-beta.1`은 미서명·미공증으로 준비한다. 릴리스 업로드와 게시는 검증 및 별도 승인 후 진행한다. 아래의 전환 전 분석은 그 당시의 기준선으로 남기되, 배포 조건은 이 결정과 [설치 안내](../README.md#install)를 따른다.

## 결론부터

Go 전환으로 확실히 얻을 수 있는 것은 **refactor-me 자체를 실행하기 위한 Node.js 24 요구를 제거할 수 있다는 점**이다. 실행 파일 하나로 컨트롤러를 배포하고, 타입과 오류 경계를 Go 코드에서 명시할 수도 있다. 다만 Git, Codex/Claude CLI, 대상 프로젝트의 빌드 도구는 계속 필요하다. provider CLI 자체의 설치 요건까지 Go 전환으로 없어지는 것은 아니다.

가장 큰 비용은 `.mjs` 문법을 Go로 바꾸는 작업이 아니다. 현재 컨트롤러가 보장하는 **원본 checkout 격리, provider 권한과 실패 분류, timeout 뒤 자식 프로세스 정리, 상태 파일의 내구성, 과거 보고서 읽기, 안전한 재설치**를 새 구현에서 입증해야 한다. 이 계약을 통과하지 못하면 바이너리의 크기나 시작 속도가 좋아져도 전환하지 않는 편이 맞다. 내부 연산이 전체 실행 시간의 병목인지도 아직 측정되지 않았다. [기존 선택지 조사](tech-stack-migration-research.ko.md)

## 1. 현재 기준선과 유지할 계약

`refactor-me`는 `run`(기본값), `doctor`, `report`, `clean`, `version`, `help`를 제공한다. `--target`, `--provider`, `--fallback`, `--json`, `--lang`, `--no-live-probe`가 주요 옵션이다. `help`와 `version`은 Git 저장소 밖에서도 성공해야 한다. 종료 코드는 `0`(완료 또는 부분 완료), `2`(실행 전 중단·CLI 오류), `4`(안전 불변식 위반)다. 부분 완료의 세부 상태는 종료 코드만으로 구별할 수 없으므로 보고서를 봐야 한다. 근거: [CLI entrypoint](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/bin/refactor-me.mjs), [사용·종료 코드 설명](../tool/README.md#usage-and-exit-codes), [CLI 테스트](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/test/cli.test.mjs).

| 경계 | 현재 동작 | Go 전환 시 보존할 결과 |
| --- | --- | --- |
| 설치 | `install.mjs`가 소스를 `.refactor/lib`에 복사하고 `.refactor/bin/refactor-me` 셸 shim 생성 | 대상 저장소별 실행 명령, 기존 `config.json`·`runs/`·`last-run.json` 보존, 알 수 없는 사용자 파일 덮어쓰기 거부 |
| 설정 | `config.json`의 기존 필드를 기본값과 병합; `commands.json`의 locked 검증 명령 사용 | 기존 설정과 잠긴 검증 명령 읽기. 알 수 없는 값이나 손상된 파일의 오류 의미를 별도 fixture로 기록 |
| 영속 기록 | `state.json`, phase JSON, `report.json`, `report.md`, patch를 run 디렉터리에 기록 | 과거 `report.json`을 읽고 `report --lang en|ko`를 출력. `report --json`은 저장된 JSON 원문을 변경 없이 출력 |
| Git 격리 | 원본 HEAD·index·작업 파일을 보존하고 별도 detached worktree에서 작업; 승인된 결과를 로컬 ref에 compare-and-swap으로 발행 | 원본 보호와 충돌 시 안전 정지, 미완료·안전 정지 worktree 보존 |
| 실행 제어 | provider·검증 명령을 셸 없이 argv로 실행; timeout 뒤 프로세스 그룹에 SIGTERM, 10초 뒤 SIGKILL | 실패·timeout·실행 불가를 성공과 구분하고 남은 자식 프로세스를 정리 |

근거: [설치기](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/install.mjs), [설정](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/src/config.mjs), [상태·잠금](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/src/state.mjs), [Git 작업](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/src/git.mjs), [provider](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/src/provider.mjs), [검증 실행기](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/src/validate.mjs), [보고서](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/src/report.mjs).

**호환성의 한계.** 이 설계는 *과거 기록 읽기*와 주요 명령·종료 코드의 의미를 목표로 한다. 모든 내부 phase JSON 필드를 영원히 동일하게 쓰겠다는 약속은 아니다. 새 run 형식이 바뀌면 버전을 명시하고, 과거 기록은 변환하거나 덮어쓰지 않고 읽기 어댑터로 처리한다. 현재 provider 세션은 재개하지 않는 설계이므로, 중단된 Node run을 Go로 이어 실행하는 기능도 범위 밖이다. [provider 설계](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/src/provider.mjs)

## 2. 준비할 것

### 계약과 테스트 자료

1. **기준선 보관:** Node 24에서 `node --test tool/test/*.test.mjs`, `help`, `version --json` 결과와 CI 조건을 기록한다. 전환 설계 당시 Node 테스트 257개가 기준선이었다. 첫 Beta 준비 당시 CI는 Node와 Go 검증을 함께 수행했다. [당시 CI](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/.github/workflows/verify.yml)
2. **비밀 없는 fixture:** 구버전 `config.json`, `commands.json`, `last-run.json`, `report.json`, 정상·손상 상태를 테스트 전용 임시 Git 저장소에 복제한다. 실제 run 디렉터리에는 prompt, provider 출력, 경로 또는 민감 정보가 있을 수 있으므로 그대로 테스트 자산에 넣지 않는다.
3. **차이 비교 항목:** CLI stdout/stderr와 exit code, 설정 기본값 병합, `report --json`의 바이트 일치, 영문·국문 보고서의 의미, Git 변경 전후의 HEAD·index·작업 파일, timeout·quota·auth·schema 오류 분류를 각각 비교한다.
4. **실행 환경:** Go 1.27 module을 CI와 맞추고, 표준 라이브러리만 사용한다. 외부 패키지가 꼭 필요하면 목적·라이선스·업데이트 책임을 검토한 뒤 별도 승인한다. `darwin/arm64`의 실제 실행 테스트는 릴리스 조건이다. [Go module 문서](https://go.dev/doc/tutorial/create-module)
5. **공개 배포 준비:** archive checksum, 빌드 출처, 태그·바이너리 버전 일치, 새 macOS 호스트의 실행 결과를 확인한다. 첫 Beta는 서명·공증을 하지 않는 것으로 결정했다. 이로 인한 macOS 경고·차단 가능성과 개별 앱 열기 안내를 사용자 문서와 릴리스 노트에 명시한다. 이후 서명된 릴리스를 도입한다면 Developer ID와 notarization 절차를 별도 작업으로 검증한다. [Apple 공식 안내](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution)

Go 개발 빌드는 `dev`로 표시하며 첫 공개 Go Beta 후보는 `0.9.20-beta.1`이다. 게시 여부는 릴리스 검증 뒤 결정한다.

## 3. 가장 많이 바뀌는 부분

### 설치와 업데이트 — 배포 계약 자체가 바뀐다

기존 Node 설치기는 소스를 복사하고 셸 shim을 만들었다. Go 설치기는 압축파일에서 꺼낸 실행 파일을 대상 저장소의 `.refactor/bin/refactor-me`로 복사한다. 대상 저장소에는 Go toolchain이 필요하지 않다. `config.json`, `runs/`, `last-run.json`과 사용자 파일은 보존한다. 이미 있는 `refactor-me`가 식별된 Node shim이나 소유 marker가 일치하는 Go 바이너리가 아니면 교체하지 않는다. 설치 실패 시 기존 명령을 복원할 수 있도록 임시 파일과 백업을 사용한다. 기존 Node 파일의 소유 여부도 파일별로 판정하는 것이 첫 릴리스 조건이다. [Go 설치기](https://github.com/soom-kang/refactor-me/blob/c19b4e212563a774b532239af6ff774f6e1ae00c/tool/go/internal/surface/install.go), [Node 설치·재설치 테스트](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/test/cli.test.mjs)

### 하위 프로세스 — `CommandContext`만으로 충분하지 않다

Node 구현은 provider와 검증 명령을 별도 프로세스 그룹으로 시작하고 그룹 전체에 SIGTERM을 보낸 뒤 10초 후 SIGKILL을 보낸다. Go의 `os/exec`는 셸을 호출하지 않는다는 점에서 argv 계약과 맞지만, `CommandContext`의 기본 취소는 프로세스 자체의 `Kill`이다. **자식·손자 프로세스까지 정리하는 현재 의미는 별도 프로세스 그룹 처리와 테스트가 필요하다.** 표준 출력 JSONL 파싱, stderr 분리, stdin 전달, timeout과 spawn 실패의 구분, provider 환경 변수 allowlist도 함께 이식해야 한다. 특히 환경 값 자체를 보고서·로그·fixture에 넣으면 안 된다. [Go `os/exec`](https://pkg.go.dev/os/exec), [현 provider 실행](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/src/provider.mjs), [현 검증 실행](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/src/validate.mjs)

### Git, 상태와 정책 — 코드량보다 의미의 검증 비용이 크다

Git은 라이브러리의 단순 치환 대상이 아니다. 현재 `git`의 argv·작업 디렉터리·출력·오류를 판정하고, 분리 worktree의 tree가 검토한 tree와 일치할 때만 ref를 compare-and-swap으로 갱신한다. Go 구현은 같은 실패 경계와 rollback 범위를 입증해야 한다. 동시에 `.refactor/lock.json`의 배타적 생성·소유자 확인과 `state.json`의 임시 파일 기록→`fsync`→rename을 보존해야 한다. 기존 상태의 `schema`와 `toolVersion`은 다른 의미이므로 합치지 않는다. [Git 계약](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/src/git.mjs), [상태 저장](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/src/state.mjs), [worktree 게이트 테스트](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/test/worktree-gate.test.mjs)

`prompts.mjs`, `schemas.mjs`, `gate.mjs`, `loop.mjs`에는 모델 출력 계약과 단계별 중단 정책이 들어 있다. Go로 옮길 때 정적 타입만으로 외부 JSON 입력을 신뢰해서는 안 된다. 응답 schema 검증, repair 횟수, 쓰기 가능 phase의 fail-closed 판정, provider 전환 조건, 검증 baseline의 `GREEN/RED/TIMEOUT/UNRUNNABLE/OPAQUE` 구분을 fixture로 비교한다. [provider 테스트](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/test/provider.test.mjs), [workflow 테스트](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/test/workflow.test.mjs), [검증 테스트](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/test/validate.test.mjs)

## 4. 기대 이득과 지불할 비용

| 항목 | 얻는 것 | 비용·단점 | 확인 방법 |
| --- | --- | --- | --- |
| 사용자 설치 | refactor-me 자체의 Node 24 선행 설치 제거; 대상 저장소에 Go 설치 불필요 | 압축파일 선택·검증, 바이너리 교체·복구 절차가 새 운영 책임 | 깨끗한 macOS ARM 환경의 설치→재설치→제거 실습 |
| 코드 안정성 | Go 타입·명시적 오류 반환으로 내부 경계를 드러내기 쉬움 | JavaScript의 동적 JSON과 provider 응답을 모델링·검증하는 코드 및 테스트 증가 | 구버전 fixture와 malformed 입력을 동일하게 분류 |
| 실행 속도·자원 | 내부 CLI 시작·파싱 비용을 줄일 가능성 | 실제 run은 모델 응답·Git·대상 프로젝트 검증을 기다릴 수 있어 총시간 이득 미확인 | 외부 대기와 controller 내부 시간을 분리해 동일 작업 비교 |
| 배포·보안 | 플랫폼별 바이너리와 릴리스 산출물의 버전 추적 가능 | 첫 Beta의 미서명 배포 위험, CI 빌드·압축파일 검증·공급망 관리 추가. 서명·공증 도입은 후속 비용 | 배포물 검증과 새 macOS 호스트 실행 |
| 유지보수 | 단일 Go runtime과 표준 도구로 개발 가능 | Node 테스트·문서·설치 경로를 한동안 병행; 버그 수정이 두 구현에 중복될 수 있음 | 병행 기간과 제거 기준을 정하고 차이 목록 관리 |

비용을 시간이나 금액으로 단정하지 않는다. Go 구현은 있지만 대표 run의 성능 프로파일은 아직 없다. 높은 위험의 프로세스 제어·Git 격리·설치 안전성에 검증 비용을 우선 배정하고, 전체 run 성능은 **모델 호출 시간 / 프로젝트 검증 시간 / controller 내부 시간**으로 나눠 측정해야 한다. 한 번의 wall time만으로 언어의 성능 차이를 주장하지 않는다.

## 5. 사용자가 받게 될 최종 형태

첫 공개 릴리스 후보는 `darwin/arm64`만 대상으로 한다. Intel, Linux, Windows 지원은 각각 설치·프로세스·Git 동작을 시험한 뒤 결정한다. Go 소스는 `tool/go/` module에 구현했고 `cmd/refactor-me`가 실행 진입점이다. 다음은 릴리스 산출물의 **예정 구조**이며, 공개 게시 완료를 뜻하지 않는다.

```text
GitHub release v0.9.20-beta.1
├── refactor-me_0.9.20-beta.1_darwin_arm64.zip
│   ├── refactor-me        # Go 실행 파일; install/uninstall과 CLI 명령 제공
│   ├── LICENSE
│   ├── INSTALL.md
│   └── BUILD-INFO.txt
└── SHA256SUMS             # 배포 파일 무결성 확인용

target-repo/.refactor/
├── bin/refactor-me        # 설치된 Go 실행 파일
├── bin/.refactor-me.install.json # 설치 바이너리의 소유 확인용 hash
├── lib/                   # 식별되지 않은 기존 파일이 있다면 보존
├── config.json            # 기존 사용자 설정 유지
├── commands.json          # 있으면 유지
├── last-run.json          # 기존 최신 run 포인터 유지
└── runs/                  # 기존 및 새 실행 기록 유지
```

게시 후 사용자는 `SHA256SUMS`로 압축파일을 검증하고, 압축을 풀어 `version --json`을 확인한 다음 `install`을 실행한다. `install`은 대상이 Git 저장소 루트인지 확인하고, 소유를 알 수 없는 기존 명령은 교체하지 않는다. 알려진 Node shim을 교체할 때도 Node 파일별 소유를 확인하지 못한 파일은 보존하고 경로를 알리는 것이 릴리스 조건이다.

`SHA256SUMS`는 다운로드한 파일의 변경을 확인하지만, 게시자 신원을 단독으로 증명하지 않는다. 첫 Beta는 미서명·미공증이므로 macOS의 경고나 차단이 발생할 수 있다. 사용자에게는 [Apple의 개별 앱 열기 안내](https://support.apple.com/en-gb/102445)를 연결하고 시스템 전체 보안 설정을 끄도록 안내하지 않는다.

```bash
./refactor-me install /path/to/target-repo
cd /path/to/target-repo
./.refactor/bin/refactor-me version --json
./.refactor/bin/refactor-me doctor --no-live-probe
./.refactor/bin/refactor-me report --lang ko
```

`doctor --no-live-probe`는 provider 모델 호출을 건너뛰지만 진단 기록을 쓸 수 있으므로 읽기 전용 명령으로 소개하지 않는다. 실제 `run`은 현재와 같이 별도 worktree에서 일하고 승인된 결과만 로컬 branch에 둔다. 기존 `report --json`은 `last-run.json`이 가리키는 **저장된 `report.json` 원문**을 그대로 내보낸다. Go가 만드는 새 기록은 형식을 버전 관리하되 과거 JSON을 자동 재작성하지 않는다. `version --json`의 Node 전용 `node` 필드는 Go에서 사실처럼 채우지 않는다. 이를 사용하는 소비자가 있는지 조사한 뒤 새 runtime 표시와 호환성 변경을 명시한다.

업데이트는 새 릴리스의 checksum과 버전을 확인한 뒤 같은 `install` 절차로 수행한다. `uninstall`은 hash가 일치하는 Go 실행 파일과 소유 marker만 제거하고 설정·실행 기록·결과 branch·증거용 worktree는 유지한다. Node로 되돌릴 때는 `.refactor/lib/src`와 `lib/bin`의 사용자 파일 충돌을 먼저 확인하고, Go `uninstall` 후 검증된 이전 Node 태그의 설치기를 사용한다. 이전 Node 설치기는 해당 디렉터리를 교체하므로 이 충돌 확인을 생략하면 안 된다. 복구 경로는 공개 전에 임시 저장소에서 시험한다.

## 6. 구현 기록과 남은 릴리스 조건

| 단계 | 상태 | 공개 전 확인할 것 |
| --- | --- | --- |
| A. 계약 고정 | Node 테스트를 기준선으로 유지 | Go 출력·종료 코드와 과거 기록 호환성 재확인 |
| B. Go 명령 | `tool/go`에서 주요 명령 구현 | 전체 Go 테스트·race·vet와 구버전 fixture 통과 |
| C. 설치와 배포물 | Go 설치·제거 구현; 첫 ZIP 제작 준비 | 사용자 파일 보존, 실패 중 복구, checksum·버전·아키텍처 검사 |
| D. 실행 컨트롤러 | provider, Git, 검증, 보고서의 Go 구현 | 프로세스 그룹 timeout, 원본 보호, hook 및 ref 충돌 시험 |
| E. 릴리스 검증 | Node와 Go 병행 검증 후 게시 | Codex·Claude 실호출, 새 macOS ARM 호스트 설치, 원격 CI, 별도 게시 승인 |

전환 시 기존 Node 출력을 무조건 정답으로 삼지 않고 문서화된 계약과 안전 정책에 비춰 차이를 판정했다. Go 커밋 경로는 hook을 실행하며, hook 이후 tree가 검토·검증된 tree와 다르면 ref 발행 없이 안전 정지해야 한다. [Go Git 구현](../tool/go/internal/workspace/git.go)

**게시 중단 조건:** 원본 checkout 변경, 구버전 보고서 손상, subprocess 실패·timeout의 성공 오분류, 설치 실패 후 기존 명령 손상, 사용자 파일 삭제, 태그·바이너리 버전 불일치가 있으면 공개하지 않는다. live provider 확인은 로컬 fixture·테스트와 별도 결과로 보고한다. 첫 Beta의 서명·공증은 진행하지 않으며, 공개 업로드와 기존 사용자 설치 교체는 별도 승인을 받은 뒤 수행한다.

## 근거와 검증 범위

- **저장소에서 확인:** 위 링크의 Node 기준선 코드와 현재 Go 코드·테스트·CI, 기존 [기술 스택 조사](tech-stack-migration-research.ko.md).
- **공식 자료에서 확인:** Go의 [빌드](https://go.dev/doc/tutorial/compile-install), [`os/exec`](https://pkg.go.dev/os/exec), Apple의 [notarization 요건](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution).
- **릴리스 검증 필요:** archive checksum·버전·아키텍처, 새 macOS 호스트 설치, provider live run, 원격 CI. 서명·notarization은 첫 Beta 범위에서 제외됐다. 실제 시간·메모리·파일 크기 개선은 측정하지 않았다.
