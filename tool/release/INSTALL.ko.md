# refactor-me 설치

[English](INSTALL.md) · [사용법](../TUTORIAL.ko.md)

이 Beta는 macOS Apple Silicon(`darwin/arm64`)만 지원합니다. Homebrew로 전역 실행 파일을 설치하고 sharpen-me Skills는 따로 설치합니다.

공개 릴리스 [`0.10.0-beta.2`](https://github.com/soom-kang/refactor-me/releases/tag/v0.10.0-beta.2)의 commit은 `c30dbdadc4229958e29366fdc14d79df3da8cb02`입니다. 공개 태그와 Homebrew formula의 [설치 검증](https://github.com/soom-kang/refactor-me/actions/runs/36956289983)을 확인할 수 있습니다.

## 이미 압축을 푼 경우

압축파일에 포함된 `INSTALL.md`를 읽고 있다면 해당 압축파일의 실행 파일을 사용하세요. 아래 Homebrew 설치와 공개 버전 다운로드 단계는 건너뜁니다. 압축을 푼 디렉터리에서 실행합니다.

```sh
cat BUILD-INFO.txt
./refactor-me version --json
```

동봉한 `version`, `commit`을 JSON 출력과 비교해 두 값이 모두 같은지 확인하세요. 실행 파일의 이름은 `refactor-me`, 플랫폼은 `darwin`, 아키텍처는 `arm64`여야 하며 `BUILD-INFO.txt`와도 같아야 합니다. 공개 릴리스 압축파일에는 `dirty=false`, `signed=no`, `notarized=no`가 기록되어 있어야 합니다. 하나라도 다르면 중단합니다. 실행 파일은 대상 저장소 밖에 보관하고 이후 명령에는 절대 경로를 사용하세요.

로컬에서 빌드한 후보에는 그 후보의 버전, commit과 dirty 상태가 들어 있습니다. 이 정보가 릴리스 게시를 뜻하지는 않습니다. 공개 Beta.2 압축파일이라면 아래 절차에 따라 commit이 릴리스 태그와 같은지도 확인하세요.

checksum은 압축파일의 바이트를 검사합니다. `BUILD-INFO.txt` 비교는 실행 파일과 메타데이터의 불일치를 찾으며 게시자를 인증하지 않습니다. 이후 웹이나 저장소의 문서를 갱신해도 동봉한 안내는 바뀌지 않습니다.

## Homebrew

Git, 인증을 마친 Codex 또는 Claude Code CLI, 대상 프로젝트의 검증 도구를 준비하세요. Node.js는 Skill 설치기에 필요하며 refactor-me 실행에는 필요하지 않습니다.

```sh
brew tap soom-kang/refactor-me
brew trust --formula soom-kang/refactor-me/refactor-me
brew install refactor-me
refactor-me version --json
npx skills add soom-kang/sharpen-me \
  --global --skill '*' --agent codex claude-code
```

Homebrew는 고정한 릴리스 소스를 Go로 빌드합니다. 필수 Skills는 `~/.agents/skills`에서 읽습니다. Homebrew는 Skills나 프로젝트 설정을 설치하지 않습니다.

Homebrew 6 이상에서 tap 등록과 해당 formula의 trust 설정은 처음 한 번만 합니다. 새 환경에서는 먼저 설정해야 `brew install refactor-me`가 외부 formula를 찾을 수 있습니다. 해당 formula만 신뢰하면 되며 tap 전체의 trust 설정은 필요하지 않습니다.

버전 `0.10.0-beta.2`와 릴리스 commit을 확인한 뒤 계속하세요. 이전 버전이라면 먼저 업데이트합니다. 선택한 provider마다 명령 옵션이나 프로젝트 설정으로 모델을 지정합니다. 고정 기본 모델은 없습니다. `doctor --no-live-probe`에는 모델이 필요하지 않으며 모델을 호출하지 않습니다. 깨끗한 Git 저장소를 선택하고 제한을 설정한 뒤 `run`을 시작하세요. 기본 doctor 검사와 `run`은 모델을 호출해 provider 사용량을 소비합니다.

## 직접 다운로드

새 빈 디렉터리를 준비합니다. [GitHub 릴리스](https://github.com/soom-kang/refactor-me/releases/tag/v0.10.0-beta.2)의 압축파일과 checksum을 그 디렉터리에 내려받으세요.

```sh
release_url=https://github.com/soom-kang/refactor-me/releases/download
archive=refactor-me_0.10.0-beta.2_darwin_arm64.zip
curl -fLO "$release_url/v0.10.0-beta.2/$archive"
curl -fLO "$release_url/v0.10.0-beta.2/SHA256SUMS"
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
  'refs/tags/v0.10.0-beta.2^{}'
```

마지막 명령은 annotated [릴리스 태그](https://github.com/soom-kang/refactor-me/tree/v0.10.0-beta.2)가 가리키는 commit과 peeled ref를 출력합니다. 첫 번째 값이 `BUILD-INFO.txt`와 JSON 출력의 `commit` 모두와 같아야 합니다. 버전은 `0.10.0-beta.2`, 플랫폼은 `darwin`, 아키텍처는 `arm64`여야 하며 메타데이터에는 `dirty=false`가 있어야 합니다. 태그가 없거나 값이 다르면 중단하세요. Homebrew로 설치한 경우에도 `refactor-me version --json`의 commit을 같은 태그와 비교합니다.

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
