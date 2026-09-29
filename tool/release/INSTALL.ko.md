# refactor-me 설치

[English](INSTALL.md) · [사용법](../TUTORIAL.ko.md)

공개 Beta는 macOS Apple Silicon을 지원합니다. Homebrew로 전역 실행 파일을 설치하고 sharpen-me Skills는 따로 설치합니다.

## Homebrew

Git, 인증을 마친 Codex 또는 Claude Code CLI, 대상 프로젝트의 검증 도구를 준비하세요. Node.js는 Skill 설치기에 필요하며 refactor-me 실행에는 필요하지 않습니다.

```sh
brew install soom-kang/refactor-me/refactor-me
refactor-me version --json
npx skills add soom-kang/sharpen-me \
  --global --skill '*' --agent codex claude-code
```

Homebrew는 고정한 릴리스 소스를 Go로 빌드합니다. 필수 Skills는 `~/.agents/skills`에서 읽습니다. Homebrew는 Skills나 프로젝트 설정을 설치하지 않습니다.

이후 [사용법](../TUTORIAL.ko.md)에 따라 깨끗한 Git 저장소를 선택하고 모델 호출 없는 doctor 검사와 제한 설정을 마친 뒤 `run`을 시작하세요. 기본 doctor 검사와 `run`은 모델을 호출해 provider 사용량을 소비합니다.

## 직접 다운로드

`0.10.0-beta.1`을 내려받으려면 새 빈 디렉터리를 준비합니다. [GitHub 릴리스](https://github.com/soom-kang/refactor-me/releases/tag/v0.10.0-beta.1)의 압축파일과 checksum을 그 디렉터리에 내려받습니다.

```sh
release_url=https://github.com/soom-kang/refactor-me/releases/download
archive=refactor-me_0.10.0-beta.1_darwin_arm64.zip
curl -fLO "$release_url/v0.10.0-beta.1/$archive"
curl -fLO "$release_url/v0.10.0-beta.1/SHA256SUMS"
awk -v file="$archive" '$2 == file {print}' SHA256SUMS | shasum -a 256 -c -
```

checksum 결과가 `OK`일 때만 압축을 풀고 실행 파일을 확인하세요.

```sh
unzip "$archive"
./refactor-me version --json
```

실행 파일은 대상 저장소 밖에 보관하고 Homebrew CLI와 같은 명령에 절대 경로를 사용합니다. 버전 `0.10.0-beta.1`, 플랫폼 `darwin`, 아키텍처 `arm64`, commit `cf31fdf797a68d6bf5cab8a0f22eff9b55766071`을 확인하세요. [릴리스 태그](https://github.com/soom-kang/refactor-me/tree/v0.10.0-beta.1)가 가리키는 commit입니다.

checksum은 다운로드 파일의 변경 여부를 확인하며 게시자를 인증하지 않습니다. 이 Beta는 Apple 서명·공증을 제공하지 않습니다. macOS가 실행을 차단하면 [Apple의 개별 앱 실행 안내](https://support.apple.com/en-gb/102445)를 따르세요. 시스템 전체 보안 설정은 유지합니다.

## 업데이트와 제거

```sh
brew upgrade soom-kang/refactor-me/refactor-me
```

업데이트는 모든 프로젝트가 사용하는 CLI를 바꿉니다. 이후 모델 호출 없는 doctor 검사를 반복하세요. 전역 Skills는 실행 사이에 따로 관리합니다.

```sh
brew uninstall refactor-me
```

제거 후에도 프로젝트 설정, 보고서, worktree, 결과 branch, 전역 Skills는 남습니다.
