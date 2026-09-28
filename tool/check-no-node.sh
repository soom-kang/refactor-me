#!/bin/sh
# Execute a check with project-level JavaScript runtimes/package managers blocked.
# GitHub Actions themselves may use the runner's bundled JavaScript runtime.
set -eu
if [ "$#" -eq 0 ]; then
  echo "usage: check-no-node.sh command [args...]" >&2
  exit 2
fi
guard_dir=$(mktemp -d)
cleanup() {
  for name in node nodejs npm npx yarn pnpm bun attempts; do
    rm -f "$guard_dir/$name"
  done
  rmdir "$guard_dir"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
export REFACTOR_NODE_ATTEMPTS="$guard_dir/attempts"
for name in node nodejs npm npx yarn pnpm bun; do
  cat > "$guard_dir/$name" <<'BLOCK'
#!/bin/sh
printf '%s\n' "${0##*/}" >> "$REFACTOR_NODE_ATTEMPTS"
echo "JavaScript tooling is disabled for this check" >&2
exit 127
BLOCK
  chmod +x "$guard_dir/$name"
done
PATH="$guard_dir:$PATH"
export PATH
status=0
"$@" || status=$?
if [ -s "$REFACTOR_NODE_ATTEMPTS" ]; then
  echo "Unexpected JavaScript tooling invocation:" >&2
  cat "$REFACTOR_NODE_ATTEMPTS" >&2
  exit 1
fi
exit "$status"
