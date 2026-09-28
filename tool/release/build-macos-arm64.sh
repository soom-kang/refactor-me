#!/bin/bash
set -euo pipefail

root=$(cd "$(dirname "$0")/../.." && pwd -P)
version=$(cat "$root/tool/RELEASE_VERSION")
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+-beta\.[1-9][0-9]*$ ]]; then
  echo "invalid tool/RELEASE_VERSION" >&2
  exit 2
fi
if ! grep -Fqx "## $version" "$root/tool/CHANGELOG.md"; then
  echo "changelog has no section for $version" >&2
  exit 2
fi

if [[ "${1:-}" == "--check-tag" ]]; then
  tag=${2:-}
  if [[ "$tag" != "v$version" ]]; then
    echo "release tag $tag does not match tool/RELEASE_VERSION ($version)" >&2
    exit 2
  fi
  exit 0
fi

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <output-directory>" >&2
  exit 2
fi
if [[ $(uname -s) != Darwin || $(uname -m) != arm64 ]]; then
  echo "release build requires macOS Apple Silicon" >&2
  exit 2
fi
if [[ -n ${RELEASE_TAG:-} && ${RELEASE_TAG} != "v$version" ]]; then
  echo "release tag does not match tool/RELEASE_VERSION" >&2
  exit 2
fi

commit=$(git -C "$root" rev-parse HEAD)
if [[ ! "$commit" =~ ^[0-9a-f]{40}$ ]]; then
  echo "cannot determine source commit" >&2
  exit 2
fi
dirty=false
if [[ -n $(git -C "$root" status --porcelain --untracked-files=normal) ]]; then
  dirty=true
fi
if [[ "$dirty" == true && ${RELEASE_CANDIDATE:-0} != 1 ]]; then
  echo "source checkout is dirty; set RELEASE_CANDIDATE=1 only for local preflight" >&2
  exit 2
fi
if [[ ${RELEASE_CANDIDATE:-0} != 1 ]]; then
  local_tag=$(git -C "$root" tag --list "v$version")
  if [[ -n "$local_tag" && $(git -C "$root" rev-list -n 1 "v$version") != "$commit" ]]; then
    echo "local release tag points to another commit" >&2
    exit 2
  fi
fi

out_dir=$1
mkdir -p "$out_dir"
out_dir=$(cd "$out_dir" && pwd -P)
archive="refactor-me_${version}_darwin_arm64.zip"
if [[ -e "$out_dir/$archive" || -e "$out_dir/SHA256SUMS" ]]; then
  echo "release output already exists; use a fresh output directory" >&2
  exit 2
fi
stage=$(mktemp -d)
trap 'if [[ -d "$stage" ]]; then rm -r "$stage"; fi' EXIT

(
  cd "$root/tool/go"
  CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -buildvcs=true \
    -ldflags "-X main.version=$version" -o "$stage/refactor-me" ./cmd/refactor-me
)

build_info=$(go version -m "$stage/refactor-me")
if ! grep -Fq "vcs.revision=$commit" <<< "$build_info"; then
  echo "binary does not embed the source commit" >&2
  exit 2
fi
if ! grep -Fq "vcs.modified=$dirty" <<< "$build_info"; then
  echo "binary VCS dirty flag differs from the source checkout" >&2
  exit 2
fi
if [[ $(lipo -archs "$stage/refactor-me") != arm64 ]]; then
  echo "binary is not darwin/arm64" >&2
  exit 2
fi
python3 -c 'import json,subprocess,sys; d=json.loads(subprocess.check_output([sys.argv[1], "version", "--json"])); assert (d["name"],d["version"],d["platform"],d["arch"]) == ("refactor-me",sys.argv[2],"darwin","arm64")' "$stage/refactor-me" "$version"

cp "$root/LICENSE" "$stage/LICENSE"
cp "$root/tool/release/INSTALL.md" "$stage/INSTALL.md"
cat > "$stage/BUILD-INFO.txt" <<EOF
name=refactor-me
version=$version
platform=darwin
arch=arm64
commit=$commit
go=$(go version | awk '{print $3}')
signed=no
notarized=no
dirty=$dirty
EOF
(
  cd "$stage"
  zip -X -q "$out_dir/$archive" refactor-me LICENSE INSTALL.md BUILD-INFO.txt
)
(
  cd "$out_dir"
  shasum -a 256 "$archive" > SHA256SUMS
  shasum -a 256 -c SHA256SUMS
)
echo "$out_dir/$archive"
echo "$out_dir/SHA256SUMS"
