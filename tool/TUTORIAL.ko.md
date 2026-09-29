# 첫 리팩토링 실행하기

[프로젝트](../docs/README.ko.md) · [English](TUTORIAL.md) · [명령·설정](README.ko.md)

macOS Apple Silicon에서 아래 다섯 단계를 따르세요. `/path/to/target-repo`를 대상 저장소 경로로 바꾸고, 공백이 있는 경로는 따옴표로 감쌉니다.

<a id="prepare-the-target"></a>

## 1. 저장소 준비

커밋이 하나 이상 있는 Git 저장소를 사용합니다. 추적하지 않는 파일까지 포함해 기존 작업을 마무리하거나 따로 보관하세요. 프로젝트 의존성을 설치하고 빌드·테스트도 한 번 실행합니다.

```sh
cd /path/to/target-repo
git rev-parse --show-toplevel
git status --short
```

`git status --short`의 출력이 없어야 합니다. refactor-me 실행 중에도 원본 checkout을 변경하지 마세요.

사용할 provider CLI의 안내에 따라 설치와 인증을 마친 뒤 상태를 확인합니다.

```sh
codex login status
# Claude Code를 쓴다면:
claude auth status
```

<a id="install-the-tool-and-skills"></a>

## 2. CLI와 Skills 설치

Homebrew와 Skill 설치기에 필요한 Node.js를 준비합니다. Homebrew가 CLI 빌드에 필요한 Go를 관리합니다. 설치된 CLI 실행에는 Go나 Node가 필요하지 않습니다.

```sh
brew install soom-kang/refactor-me/refactor-me
refactor-me version --json
npx skills add soom-kang/sharpen-me \
  --global --skill '*' --agent codex claude-code
```

CLI 버전 `0.10.0-beta.1`, 플랫폼 `darwin`, 아키텍처 `arm64`를 확인합니다. 필수 Skill 8종은 `~/.agents/skills`에서 읽습니다. 대상 프로젝트에 같은 Skill의 별도 복사본을 두면 충돌할 수 있으며 doctor가 경로를 알려줍니다.

설정을 만든 뒤 모델 호출 없이 Codex 실행 준비를 확인합니다.

```sh
refactor-me init --repo /path/to/target-repo
refactor-me doctor --repo /path/to/target-repo \
  --provider codex --fallback none --no-live-probe
```

Claude Code를 쓰려면 doctor와 run 모두에 `--provider claude --fallback none`을 사용합니다. 실행을 막는 `FAIL` 항목을 해결한 뒤 다음 단계로 넘어가세요. `init`은 기존 설정을 덮어쓰지 않습니다. 이 안내에서는 다음 단계에서 설정 파일을 수정하므로 먼저 실행하세요. 초기화를 생략하면 내장 기본값을 사용하며, 인자 없는 `refactor-me`는 도움말을 표시합니다.

`--no-live-probe`는 모델 호출을 생략하지만 provider 실행 파일을 확인하고 임시 진단 환경을 만듭니다. 실제 세션의 Skill 인식을 확인하려면 이 옵션 없이 doctor를 실행하세요. 이때 provider 사용량이 발생합니다.

<a id="set-limits-and-run"></a>

## 3. 첫 실행 제한 설정

대상 저장소의 `.refactor/config.json`을 엽니다. 기존 객체 안에서 아래 항목을 수정하세요. 전체 파일을 대체하는 예시가 아닙니다.

```json
{
  "agents": {
    "codex": {"timeout_sec": 300},
    "claude": {"timeout_sec": 300}
  },
  "policy": {
    "max_cycles": 1,
    "max_commits": 1,
    "max_wall_clock_min": 20
  }
}
```

첫 시도를 1 cycle과 리팩토링 커밋 1개로 제한하는 예시입니다. 기존 동작을 기록하는 characterization 테스트 커밋은 별도로 계산합니다. provider timeout은 호출마다 적용하며, 전체 시간은 작업 단위 사이에서 확인하므로 정확히 그 시각에 종료되지는 않습니다.

**전체 금액 상한은 없습니다.** 모델 호출, 재시도, 실제 세션을 확인하는 doctor가 계정 사용량을 소비합니다. 제한을 늘리기 전에 [전체 설정](README.ko.md#configuration)을 확인하세요.

## 4. 실행

어느 디렉터리에서든 Codex만 사용해 실행할 수 있습니다.

```sh
refactor-me run --repo /path/to/target-repo \
  --provider codex --fallback none
```

Claude Code를 쓰려면 `codex`를 `claude`로 바꿉니다. Codex 실패 시 Claude로 전환하려면 `--provider codex --fallback claude`를 사용합니다. provider 옵션을 생략해도 이 순서로 실행합니다. `--lang ko`를 추가하면 보고서와 최종 요약을 한글로 표시합니다.

특정 디렉터리에서 후보를 찾으려면 다음과 같이 실행합니다.

```sh
refactor-me run --repo /path/to/target-repo \
  --target app/web --target app/api \
  --provider codex --fallback none
```

추적 중인 소스 파일이 있는 디렉터리를 지정하세요. 저장소 전체를 조사하려면 `--target`을 생략합니다.

| 경로 | 해석 기준 |
| --- | --- |
| 상대 경로 `--repo` | 호출한 디렉터리에서 경로를 찾은 뒤 Git 루트 선택 |
| `--repo`와 함께 쓴 `--target` | 선택한 Git 루트 |
| `--repo` 없이 쓴 `--target` | 호출한 디렉터리 |

실제 경로가 저장소 밖이면 거부합니다. **target은 후보 조사 범위입니다.** 호출부 수정이나 검증 명령은 저장소 안의 다른 디렉터리까지 포함할 수 있습니다.

![준비 상태를 확인하고 격리 worktree에서 실행한 뒤, 저장된 결과와 변경 사항을 검토합니다.](../docs/assets/workflow/execution.ko.png)

<a id="read-the-result"></a>

## 5. 결과 확인

명령이 성공으로 끝나도 보고서를 읽으세요.

```sh
refactor-me report --repo /path/to/target-repo
refactor-me report --repo /path/to/target-repo --lang ko
refactor-me report --repo /path/to/target-repo --json
```

보고서 조회는 모델을 호출하지 않습니다. 언어를 바꿔 조회해도 저장된 보고서는 그대로이며, `--json`은 지원하는 저장 JSON의 원문 바이트를 출력합니다.

1. 종료 상태와 중단 이유를 확인합니다. 종료 코드 `0`에는 부분 완료도 포함됩니다.
2. 검증 결과, 생략한 검사, provider 사용량을 확인합니다. 비용이 없으면 0원이 아니라 미확인입니다.
3. 로컬 결과 branch와 `.refactor/runs/<id>/changes.patch`를 읽습니다. 발행한 변경이 없으면 patch나 결과 branch가 없을 수 있습니다.
4. diff를 검토하고 생략된 통합·브라우저 검사를 실행한 뒤 직접 병합합니다.

결과 branch는 `refactor/auto-*` 형식입니다. CLI는 자동 병합, push, 배포를 하지 않습니다. `accepted.patch`는 중간 검토 자료이며, 최종 발행한 변경은 `changes.patch`로 확인합니다.

## 중단했을 때

| 신호 | 다음 행동 |
| --- | --- |
| Skill 누락·충돌·변경 | doctor가 표시한 경로를 확인하고 실행 사이에 전역 설치 수정 |
| 원본 변경 또는 잘못된 target | 기존 작업을 마무리하거나 경로를 고친 뒤 doctor 재실행 |
| 기준선 검증 실패 | 의존성과 [검증 명령](README.ko.md#validation-commands) 확인. 최소 한 명령은 통과해야 함 |
| 변경 없음 또는 부분 완료 | 제외 이유와 실행 제한을 읽은 뒤 재실행 판단 |
| 종료 코드 `4`, 안전 중단 | worktree와 진단 기록을 보존하고 원인 조사 |

종료 코드 `2`는 실행 중단 또는 CLI 오류입니다. stderr와 남아 있는 실행 기록을 확인하세요. 설정 schema `2`, 보고서 schema `3`을 지원하며 다른 형식은 파일을 바꾸지 않고 오류로 알립니다. 보고서가 없거나 잘못되었다고 해서 실행이 성공한 것은 아닙니다.

기록에는 소스 일부와 명령 출력이 들어갈 수 있으므로 공유 전에 확인하세요. 복구와 결과 발행 조건은 [동작 방식](WORKFLOW.ko.md)에 설명되어 있습니다.

## 업데이트와 제거

CLI를 업데이트한 뒤 모델 호출 없는 doctor 검사를 반복합니다.

```sh
brew upgrade soom-kang/refactor-me/refactor-me
refactor-me version --json
refactor-me doctor --repo /path/to/target-repo \
  --provider codex --fallback none --no-live-probe
```

업데이트는 이 실행 파일을 쓰는 모든 프로젝트에 적용됩니다. Skills는 로컬 수정 사항을 확인한 뒤 2단계 설치 명령으로 따로 갱신합니다. 실행 도중에는 Skills를 바꾸지 마세요.

`refactor-me clean --repo /path/to/target-repo`는 제거 조건을 충족한 완료 worktree를 정리합니다. 부분 완료나 안전 중단 worktree는 조사할 수 있도록 남깁니다.

```sh
brew uninstall refactor-me
```

제거 후에도 프로젝트 설정, 보고서, worktree, 결과 branch, 전역 Skills는 남습니다. 이 Beta는 Apple 서명·공증을 제공하지 않습니다. 직접 다운로드와 경고 대응은 [설치 안내](release/INSTALL.ko.md)를 참고하세요.
