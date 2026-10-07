# refactor-me 설치

[English](INSTALL.md) · [사용법](../TUTORIAL.ko.md)

이 Beta는 macOS Apple Silicon(`darwin/arm64`)만 지원합니다. Homebrew로 전역 실행 파일을 설치하고 sharpen-me Skills는 따로 설치합니다.

공개 릴리스 [`0.10.0-beta.5`](https://github.com/soom-kang/refactor-me/releases/tag/v0.10.0-beta.5)의 commit은 `8bfebe90b04e54cf1ef843915aad2901be0db41d`입니다. 공개 태그와 Homebrew formula의 [설치 검증](https://github.com/soom-kang/refactor-me/actions/runs/37573464996)을 통과했습니다. 아래 절차에 따라 압축파일의 버전과 commit을 `BUILD-INFO.txt`, 실행 파일의 JSON 출력, 릴리스 태그와 비교하세요.

## 이미 압축을 푼 경우

압축파일에 포함된 `INSTALL.md`를 읽고 있다면 해당 압축파일의 실행 파일을 사용하세요. 아래 Homebrew 설치와 공개 버전 다운로드 단계는 건너뜁니다. 압축을 푼 디렉터리에서 실행합니다.

```sh
cat BUILD-INFO.txt
./refactor-me version --json
```

동봉한 `version`, `commit`을 JSON 출력과 비교해 두 값이 모두 같은지 확인하세요. 실행 파일의 이름은 `refactor-me`, 플랫폼은 `darwin`, 아키텍처는 `arm64`여야 하며 `BUILD-INFO.txt`와도 같아야 합니다. 공개 릴리스 압축파일에는 `dirty=false`, `signed=no`, `notarized=no`가 기록되어 있어야 합니다. 하나라도 다르면 중단합니다. 실행 파일은 대상 저장소 밖에 보관하고 이후 명령에는 절대 경로를 사용하세요.

로컬에서 빌드한 후보에는 그 후보의 버전, commit과 dirty 상태가 들어 있습니다. 이 정보가 릴리스 게시를 뜻하지는 않습니다. 공개 Beta.5 압축파일이라면 아래 절차에 따라 commit이 릴리스 태그와 같은지도 확인하세요.

checksum은 압축파일의 바이트를 검사합니다. `BUILD-INFO.txt` 비교는 실행 파일과 메타데이터의 불일치를 찾으며 게시자를 인증하지 않습니다. 이후 웹이나 저장소의 문서를 갱신해도 동봉한 안내는 바뀌지 않습니다.

## Homebrew

Git, 인증을 마친 Codex 또는 Claude Code CLI, 대상 프로젝트의 검증 도구를 준비하세요. 아래의 Git 기반 고정 Skill 설치에는 Node.js가 필요하지 않습니다.

```sh
brew tap soom-kang/refactor-me
brew trust --formula soom-kang/refactor-me/refactor-me
brew install refactor-me
refactor-me version --json
```

Homebrew는 고정한 릴리스 소스를 Go로 빌드합니다. 필수 Skills는 `~/.agents/skills`에서 읽습니다. Homebrew는 Skills나 프로젝트 설정을 설치하지 않습니다. 첫 doctor 검사 전에 아래 [Skill 기준 카탈로그](#skill-reference)를 설치하세요.

Homebrew 6 이상에서 tap 등록과 해당 formula의 trust 설정은 처음 한 번만 합니다. 새 환경에서는 먼저 설정해야 `brew install refactor-me`가 외부 formula를 찾을 수 있습니다. 해당 formula만 신뢰하면 되며 tap 전체의 trust 설정은 필요하지 않습니다.

버전 `0.10.0-beta.5`와 실행 파일의 commit을 annotated tag의 peeled commit과 비교한 뒤 계속하세요. 이전 버전이라면 먼저 업데이트합니다. 선택한 provider마다 명령 옵션이나 프로젝트 설정으로 모델을 지정합니다. 고정 기본 모델은 없습니다. `doctor --no-live-probe`에는 모델이 필요하지 않으며 모델을 호출하지 않습니다. 깨끗한 Git 저장소를 선택하고 제한을 설정한 뒤 `run`을 시작하세요. 기본 doctor 검사와 `run`은 모델을 호출해 provider 사용량을 소비합니다.

<a id="skill-reference"></a>

## 고정 Skill 기준 카탈로그

기준은 [sharpen-me commit `fbb88aea30ff46ade265607ea6572be7cc642a4c`](https://github.com/soom-kang/sharpen-me/tree/fbb88aea30ff46ade265607ea6572be7cc642a4c)입니다. 실행 파일에 포함된 [manifest](https://github.com/soom-kang/refactor-me/blob/main/tool/go/internal/catalog/reference.json)는 `LICENSE`와 `agents/` 파일을 포함한 8개 Skill 전체 트리의 SHA-256을 기록합니다. 재현 가능한 내용 기준이며 실제 provider 호환성 검증은 `NOT_RUN`입니다.

Skills를 처음 설치할 때 아래 Git 전용 경로를 사용하세요. 변경될 수 있는 `npx` 설치기를 실행하지 않고 소스 commit을 고정합니다. 해당 이름의 Skill이나 소스 checkout이 이미 있으면 중단합니다. 기존·사용자 정의 설치는 보존하고 교체 여부를 실행 사이에 직접 검토하세요. 전역 경로가 소스를 가리키므로 소스 checkout을 유지하세요.

```sh
(
  set -eu
  revision=fbb88aea30ff46ade265607ea6572be7cc642a4c
  source="$HOME/.local/share/refactor-me/sharpen-me-$revision"
  names="sharpen-clarify sharpen-review sharpen-challenge sharpen-assess sharpen-refine sharpen-cold-review sharpen-brief sharpen-dedupe"
  if test -e "$source" || test -L "$source"; then
    echo "Existing source: $source; preserve it and resolve manually." >&2
    exit 1
  fi
  for name in $names; do
    for root in "$HOME/.agents/skills" "$HOME/.claude/skills" "$HOME/.codex/skills"; do
      if test -e "$root/$name" || test -L "$root/$name"; then
        echo "Existing Skill: $root/$name; preserve it and resolve manually." >&2
        exit 1
      fi
    done
  done
  mkdir -p "$(dirname "$source")"
  git -c core.autocrlf=false clone --no-checkout https://github.com/soom-kang/sharpen-me.git "$source"
  git -C "$source" -c core.autocrlf=false checkout --detach "$revision"
  test "$(git -C "$source" rev-parse HEAD)" = "$revision"
  mkdir -p "$HOME/.agents/skills" "$HOME/.claude/skills"
  for name in $names; do
    ln -s "$source/skills/$name" "$HOME/.agents/skills/$name"
    ln -s "$HOME/.agents/skills/$name" "$HOME/.claude/skills/$name"
  done
)
```

설치 후 `refactor-me doctor --repo /path/to/target-repo --provider codex --fallback none --no-live-probe`를 실행하세요. Claude를 쓰면 `--provider claude`로 바꿉니다. 프로젝트 복사본이나 별도 provider 설정 경로를 포함해 doctor가 보고한 탐색 충돌을 해결하세요.

새 `skill-reference` 진단을 포함한 빌드는 `PASS`(기준 내용 일치), 실행을 막지 않는 `WARN`(미검증·사용자 정의 내용, 다른 Skill 이름 표시), `FAIL`(카탈로그 사용 불가, 별도 `global-skills` 실패가 실행 차단)을 구분합니다. 유효한 사용자 정의 카탈로그와 기존 실행 중 변경 감지는 유지합니다. 내용 일치는 실제 Skill 로딩이나 모델 동작을 입증하지 않습니다. 이미 공개한 Beta.5 실행 파일에는 이 새 진단이 없으며 과거 Beta.5 검증 기록은 바뀌지 않습니다.

## 직접 다운로드

새 빈 디렉터리를 준비합니다. [GitHub 릴리스](https://github.com/soom-kang/refactor-me/releases/tag/v0.10.0-beta.5)의 압축파일과 checksum을 그 디렉터리에 내려받으세요.

```sh
release_url=https://github.com/soom-kang/refactor-me/releases/download
archive=refactor-me_0.10.0-beta.5_darwin_arm64.zip
curl -fLO "$release_url/v0.10.0-beta.5/$archive"
curl -fLO "$release_url/v0.10.0-beta.5/SHA256SUMS"
awk -v file="$archive" '$2 == file {print; count++} END {if (count != 1) exit 1}' \
  SHA256SUMS > "$archive.sha256" &&
  shasum -a 256 -c "$archive.sha256"
```

checksum 결과가 `OK`일 때만 압축을 풀고 실행 파일을 확인하세요.

```sh
unzip "$archive"
cat BUILD-INFO.txt
./refactor-me version --json
git ls-remote https://github.com/soom-kang/refactor-me.git \
  'refs/tags/v0.10.0-beta.5^{}'
```

마지막 명령은 annotated [릴리스 태그](https://github.com/soom-kang/refactor-me/tree/v0.10.0-beta.5)가 가리키는 commit과 peeled ref를 출력합니다. 첫 번째 값이 `BUILD-INFO.txt`와 JSON 출력의 `commit` 모두와 같아야 합니다. 버전은 `0.10.0-beta.5`, 플랫폼은 `darwin`, 아키텍처는 `arm64`여야 하며 메타데이터에는 `dirty=false`가 있어야 합니다. 태그가 없거나 값이 다르면 중단하세요. Homebrew로 설치한 경우에도 `refactor-me version --json`의 commit을 같은 태그와 비교합니다.

실행 파일은 대상 저장소 밖에 보관하고 Homebrew CLI와 같은 명령에 절대 경로를 사용합니다.

checksum은 다운로드 파일의 변경 여부를 확인하며 게시자를 인증하지 않습니다. 이 Beta는 Apple 서명·공증을 제공하지 않습니다. macOS가 실행을 차단하면 [Apple의 개별 앱 실행 안내](https://support.apple.com/en-gb/102445)를 따르세요. 시스템 전체 보안 설정은 유지합니다.

## 업데이트와 제거

```sh
brew upgrade refactor-me
```

업데이트는 모든 프로젝트가 사용하는 CLI를 바꿉니다. 이후 모델 호출 없는 doctor 검사를 반복하세요. 전역 Skills는 실행 사이에 따로 관리합니다.

```sh
brew uninstall refactor-me
```

제거 후에도 프로젝트 설정, 보고서, worktree, 결과 branch, 전역 Skills는 남습니다.
