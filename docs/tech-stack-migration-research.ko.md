# refactor-me CLI 기술 스택 전환 조사

> **과거 전환 자료:** 본문은 2026-09-28 조사·설계와 당시 릴리스 준비 판단을 보존한 기록입니다. 현재 개발 절차는 [Go 개발 안내](../tool/README.ko.md#로컬-개발-검사), 테스트 이관 상태는 [계약 이관표](node-test-contracts.ko.md)를 따릅니다. 아래 Node 코드 링크는 삭제 전 고정 commit을 가리키며 현재 checkout의 실행 경로가 아닙니다.
- **기준일:** 2026-09-28
- **대상:** `tool/`의 CLI와 설치기. Skills 자체, `app/web`, 실행 대상 저장소의 언어는 범위 밖.
- **상태:** 의사결정용 조사. 전환 구현, 배포, 실사용 성능 측정은 수행하지 않았다.

## 결정 요약

현재 목표가 **기존의 빌드 없는 복사 설치 유지**라면, 언어 전면 교체보다 JavaScript의 점진적 타입 검사 또는 TypeScript 전환이 비용 대비 타당하다. **Node 설치가 필요 없는 실행 파일 배포**가 제품 요구사항이라면 Go를 먼저 실험할 가치가 있다. Rust는 이 CLI에서 고유한 이득을 입증할 병목이나 메모리 안전 요구가 확인되기 전까지 전면 재작성 후보로 우선하지 않는다. 이는 측정 결과가 아니라 현재 코드와 배포 계약에 근거한 판단이다.

| 판단할 요구 | 우선 검토 | 조건 |
| --- | --- | --- |
| 현재 설치 방식과 동작을 유지하며 변경 오류 줄이기 | JSDoc 타입 검사 → TypeScript | 정적 검사 도입 비용과 기존 테스트의 회귀 탐지력을 확인 |
| 사용자에게 Node 설치를 요구하지 않기 | Go 실행 파일 | 설치기, 상태 파일, Git·provider 프로세스 계약을 보존하는 시제품 통과 |
| 실행 파일과 엄격한 시스템 언어가 모두 필요한 경우 | Rust | Go 대비 추가 이득을 실제 측정으로 확인 |

## 1. 현재 시스템에서 확인한 사실

`README.md`는 macOS, Node.js 24+, Git, 인증된 Codex 또는 Claude CLI를 전제로 한다. [`tool/install.mjs`](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/install.mjs)는 `tool/src`와 `tool/bin`을 대상 저장소의 `.refactor/lib`에 복사하고 셸 shim을 만든다. 설치에 npm 의존성 설치나 빌드는 없다. 설정과 과거 run 자료는 설치 업데이트에서 보존한다. [`tool/bin/refactor-me.mjs`](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/bin/refactor-me.mjs)가 `help`, `version`, `doctor`, 실행, `report` 등을 처리한다.

`tool/src`의 15개 `.mjs` 모듈은 Git worktree, 스코프, provider 호출, JSON schema, 정책 게이트, 검증 명령, 상태·보고서를 분담한다. `tool/test`에는 18개 `*.test.mjs` 파일이 있다. 이 숫자는 파일 수이지 테스트 케이스 수가 아니다. 전체 `tool` 소스·테스트는 약 9.4천 줄이다 (`wc -l`의 개행 기준). 별도 `package.json`, `tsconfig.json`, `go.mod`, `Cargo.toml`은 현재 확인되지 않는다.

특히 다음은 언어를 바꿔도 지켜야 할 **관찰 가능한 계약**이다.

1. 원본 checkout의 HEAD·index·작업 파일을 건드리지 않고 별도 worktree에서 변경·검증·로컬 branch 발행을 수행한다 ([`tool/src/git.mjs`](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/src/git.mjs), [`tool/src/loop.mjs`](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/src/loop.mjs)).
2. provider와 검증 명령을 셸 문자열이 아닌 argv로 실행하고, 제한된 환경 변수·timeout·프로세스 종료·오류 분류를 적용한다 ([`tool/src/provider.mjs`](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/src/provider.mjs), [`tool/src/validate.mjs`](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/src/validate.mjs)).
3. `.refactor/config.json`, `state.json`, `report.json`, phase JSON, patch와 기존 run 읽기 동작을 보존한다 ([`tool/src/config.mjs`](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/src/config.mjs), [`tool/src/state.mjs`](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/src/state.mjs), [`tool/src/report.mjs`](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/src/report.mjs)).
4. 상태 JSON을 임시 파일에 쓰고 `fsync` 후 rename한다. 잠금·재시작·실패 복구의 의미는 파일 형식뿐 아니라 파일시스템 동작에도 걸려 있다 ([`tool/src/state.mjs`](https://github.com/soom-kang/refactor-me/blob/fdf05a1a2609ef15a185479c379a4a46eeb349b4/tool/src/state.mjs)).
5. CI는 현재 macOS에서 Node 24 구문 검사, `node --test`, CLI smoke check를 실행한다 (`.github/workflows/verify.yml`). Linux/Windows 지원은 이 CI만으로 확인할 수 없다.

따라서 전환 비용의 중심은 파서 성능보다 **Git와 하위 프로세스의 실패 의미, 영속 상태의 호환성, 설치 경로**다. 현재 구현에는 macOS에 가까운 `cp -c`, `/usr/bin/which`, Unix 프로세스 그룹 신호가 있으므로 Go/Rust로 컴파일만 해서는 플랫폼 지원 범위가 넓어지지 않는다.

## 2. 선택지 비교

| 선택지 | 실질적 장점 | 실질적 단점·이행 비용 | 현 설치 방식 | 단일 실행 파일 |
| --- | --- | --- | --- | --- |
| `.mjs` 유지 + JSDoc/`checkJs` | 런타임과 설치 계약을 그대로 두고 핵심 JSON·상태 경계에 타입 검사 추가 가능. 가장 작은 실험. | 타입 표현이 장황할 수 있고 잘못된 외부 입력을 타입 검사만으로 검증하지 못함. `tsc`를 개발 도구로 운영해야 함. | 그대로 가능 | 별도 패키징 필요 |
| TypeScript → JavaScript 빌드 | 현재 Node API·모듈 구조 재사용. 정적 타입과 IDE 지원. 배포 산출물은 JS이므로 기존 shim을 유지할 수 있음. | 빌드 산출물 관리, source map, 릴리스 검증, `.mjs` import specifier 조정. 개발 의존성·잠금 파일 도입. | **빌드된 JS 복사**로 가능 | Node SEA 등 별도 설계·검증 필요 |
| Node 24 native TypeScript | erasable syntax에 한해 transpiler 없이 `.mts`를 실행할 수 있어 빌드 없는 설치에 가까움. | Node는 타입 검사도 `tsconfig` 해석도 하지 않음. `.mjs` → `.mts` import 경로와 shim 수정 필요. 설치 대상에 TypeScript 소스를 복사하며 Node 24 버전 계약에 묶임. | **`.mts` 소스 복사**로 가능 | 별도 패키징 필요 |
| Go 재작성 | Go toolchain으로 플랫폼별 실행 파일 생성. 사용자 런타임 의존성을 없앨 수 있고 명시적 타입·오류 처리가 가능. | JSON·Git·프로세스·잠금·신호·파일 권한의 의미를 재구현해야 함. 기존 JS 테스트를 곧바로 실행하지 못하며 배포 파일과 업데이트 절차를 새로 운영해야 함. | 기존 방식과 별도 설치기 필요 | 가능; 플랫폼별 빌드·시험 필요 |
| Rust 재작성 | 플랫폼별 native 실행 파일, 강한 타입·소유권 모델. 장기적으로 시스템 경계의 세밀한 제어 가능. | Go와 같은 호환성 재구현에 더해 학습·컴파일·crate/target 운영 비용. 이 프로젝트에서 성능·메모리상 이득은 아직 실측 근거 없음. | 기존 방식과 별도 설치기 필요 | 가능; target별 빌드·시험 필요 |

### TypeScript에서 구분해야 할 두 경로

Node 24의 type stripping은 stable이지만, **실행만** 담당한다. 타입 검사는 별도 `tsc --noEmit`이 필요하다. Node는 `tsconfig.json`을 읽지 않고 `enum`, parameter property처럼 런타임 변환이 필요한 구문은 기본 경로에서 지원하지 않는다. ESM을 강제하려면 `.mts`가 현재 `.mjs`와 가장 직접적으로 대응한다. 상대 import 확장자를 실제 파일에 맞게 바꿔야 한다. 공식 문서의 `erasableSyntaxOnly`, `verbatimModuleSyntax` 등 설정을 시제품에서 검증해야 한다. [Node 24 TypeScript 문서](https://nodejs.org/download/release/v24.20.0/docs/api/typescript.html)

반면 빌드형 TypeScript는 배포 시 JavaScript만 복사할 수 있어 사용자의 Node 런타임 계약을 유지하기 쉽다. 그 대신 빌드가 새로운 릴리스 필수 단계가 된다. TypeScript의 `allowJs`/`checkJs`는 그 전에 기존 JS와 TS를 함께 검사하는 점진적 경로다. [TypeScript `allowJs`](https://www.typescriptlang.org/tsconfig/allowJs.html), [TypeScript `checkJs`](https://www.typescriptlang.org/tsconfig/checkJs.html)

Node의 Single Executable Applications도 실행 파일 배포의 비교군이다. 다만 Node 24의 SEA 생성에는 blob 주입과 asset 처리 등 별도 패키징이 필요하므로 현재 소스 복사 설치와 동일한 운영 비용으로 간주할 수 없다. **Go/Rust의 비교 대상에 포함하되**, 실제 크기·시작 시간·서명·업데이트 난이도는 같은 시제품으로 측정해야 한다. [Node 24 SEA 문서](https://nodejs.org/download/release/v24.18.0/docs/api/single-executable-applications.html)

### Go와 Rust가 해결하지 않는 부분

Go의 `go build`와 Rust의 Cargo는 실행 파일을 만든다. 그러나 Codex/Claude CLI, Git, 대상 프로젝트의 테스트 도구는 여전히 외부 전제다. 또한 macOS 전용 동작을 그대로 이식하면 “바이너리 하나”가 곧 “모든 OS 지원”이 되지 않는다. Go의 순수 Go cross build는 비교적 단순하지만 cgo 사용 시 cross compiler가 필요할 수 있다. Rust는 target마다 지원 수준과 링커 요건을 확인해야 한다. [Go build/install](https://go.dev/doc/tutorial/compile-install), [Go cross compilation](https://go.dev/wiki/WindowsCrossCompiling), [Cargo build](https://doc.rust-lang.org/cargo/commands/cargo-build.html), [Rust platform support](https://doc.rust-lang.org/rustc/platform-support.html)

## 3. 전환 전 반증해야 할 가정

**가정 A — 현재 문제의 주원인은 `.mjs`의 타입 부재다.** 지금 저장소에는 오류 종류·수정 시간·실행 프로파일의 계량 자료가 없다. 타입 검사로 예방 가능한 오류가 실제로 반복됐는지 확인 전에는 전면 TS 전환의 편익을 수치화할 수 없다. 작은 관찰: `state`, `schemas`, `provider` 중 한 경계에 타입 검사만 적용하여 새로 발견된 **실제 오류**와 false positive, 유지비를 기록한다. 기존 테스트 결과와 비교하되 타입 오류 수를 제품 품질의 대리 지표로 취급하지 않는다.

**가정 B — native 재작성으로 사용자 경험이 개선된다.** 실행 시간의 대부분이 모델 응답·Git·프로젝트 검증이라면 controller 언어 변경의 총 실행 시간 효과는 작을 수 있다. 작은 관찰: 실제 run에서 phase별 wall time을 기록하고 controller 내부 시간과 외부 대기 시간을 분리한다. 모델 호출이 포함된 총 시간 한 번만 재는 방식으로는 언어 효과를 판별할 수 없다.

**가정 C — 실행 파일 배포가 현재 설치보다 쉽다.** 단일 파일의 이점은 사용자 Node 설치 제거에 있지만, macOS arch별 빌드·서명·설치 업데이트·진단과 기존 `.refactor` 자료 읽기를 새로 담당해야 한다. 작은 관찰: 깨끗한 macOS ARM64/Intel 환경에서 설치→`version`→`doctor --no-live-probe`→기존 보고서 읽기→업데이트→제거를 두 방식으로 비교한다. provider 호출은 제외해 외부 계정 상태와 설치 경험을 분리한다.

**중단 조건:** 시제품이 원본 checkout을 수정하거나, 기존 run을 잘못 읽거나, subprocess 실패·timeout을 성공으로 분류하면 해당 전환 경로의 확대를 중단한다. 이 조건은 속도 측정보다 우선한다.

## 4. 권장 검증 순서와 판정 기준

1. **기준선 고정:** 현재 checkout에서 `node --test tool/test/*.test.mjs`, `node tool/bin/refactor-me.mjs help`, `node tool/bin/refactor-me.mjs version --json`을 기록한다. 공개 계약으로 CLI 출력/exit code, 설치 후 파일, 설정과 보고서 JSON, 원본 checkout 보호를 목록화한다. 현재 테스트 통과가 provider 실사용 성공을 뜻하지는 않는다.
2. **최소 타입 실험:** 먼저 기존 `.mjs`의 한 JSON 경계에 JSDoc/`checkJs`를 적용한다. 유의미한 오류가 포착되고 유지비가 감당 가능할 때만 `.mts` native 실행과 TS→JS 빌드의 두 배포 방식을 동일 경계에서 비교한다. `tsc --noEmit`과 런타임 테스트는 각각 실행한다.
3. **실행 파일 요구가 확인되면:** Go로 `version`, `help`, config 읽기, 보고서 읽기만 하는 제한된 시제품을 만든다. 기존 run 샘플을 읽고 JSON과 exit code를 비교한다. Git worktree 생성이나 provider 호출은 이 시제품의 다음 단계이며, 문서 조사만으로 안전성을 승인하지 않는다.
4. **Rust 또는 Node SEA 비교:** 실행 파일 크기, cold start, 메모리, 설치/업데이트 단계, 대상 arch별 실패를 동일 환경에서 재고 Go와 비교한다. 측정 전 “Rust가 더 빠르다” 또는 “Go가 더 작다”는 결론을 내리지 않는다.
5. **확대 게이트:** 사용자의 목표에 맞는 개선을 관찰하고, 기존 테스트와 계약 fixture가 통과하고, macOS ARM64/Intel 설치·업데이트·복구 및 실패 경로가 검증될 때만 실제 전환 계획을 승인 대상으로 올린다. Linux/Windows를 지원 목표로 정한다면 각 OS의 Git, 프로세스 종료, 경로·권한 테스트를 별도로 추가한다.

이 순서는 **선행 조사 및 제안**이다. 의존성 설치, 언어 마이그레이션, 릴리스 파이프라인 변경, 배포는 이번 조사에 포함하지 않는다.

## 5. 조사 범위와 미확인 사항

- 확인: 현재 저장소의 README, 설치기, CLI entrypoint, 핵심 상태/Git/provider/검증 코드, 테스트 파일 목록, macOS CI 설정.
- 외부 근거: 위에 링크한 Node.js, TypeScript, Go, Rust 공식 문서. 문서의 기능 설명과 이 저장소에 적용했을 때의 비용 판단을 구분했다.
- 이번 조사에서 실행: `node --test tool/test/*.test.mjs` (257/257 통과), `node tool/bin/refactor-me.mjs help`, `node tool/bin/refactor-me.mjs version --json`, `git diff --check`. 이는 현재 Node 구현의 로컬 기준선만 확인한다.
- 미실행: TypeScript/Go/Rust 시제품, provider를 사용하는 live run, 성능·바이너리 크기·사용자 설치 시간 측정. 따라서 언어별 성능 우열과 총 개발 공수는 확정하지 않는다.
- 결정 필요: 배포 목표 두 방식을 모두 검토했지만, 실제 전환 전에 **어느 문제가 우선인지**(타입 관련 결함, 설치 마찰, 지원 OS, 실행 비용)와 허용 가능한 호환성 변경 범위를 정해야 한다.
