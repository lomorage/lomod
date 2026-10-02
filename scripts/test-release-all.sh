#!/usr/bin/env bash
# Tests scripts/release-all.sh without building or publishing anything real:
#   1. --only / --dry-run: which platforms each host releases, and what it refuses.
#   2. update-release-manifest.py: sets CLI keys, leaves every other key alone, refuses
#      LomoAgent's own.
#   3. publish_homepage against a throwaway homepage repo + bare "origin" in WSL: a plain push;
#      a push after origin moved on another key (rebased and pushed, local edits kept); and a
#      real conflict on the same key (stops, local commit kept, origin untouched).
#
# Run from native Windows git-bash: bash scripts/test-release-all.sh
set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_WSL_PATH="$(wsl -d Ubuntu -e wslpath -a "$(cygpath -w "$REPO")" | tr -d '\r')"

failures=0
ok() { echo "  ok   $1"; }
fail() { echo "  FAIL $1"; failures=$((failures + 1)); }
check() { if eval "$1"; then ok "$2"; else fail "$2"; fi; }
inwsl() { wsl -d Ubuntu -e bash -c "$1"; }

echo "==> 1. --only / --dry-run"
plan() { RELEASE_HOST="$1" bash "$REPO/scripts/release-all.sh" "${@:2}" --dry-run 2>&1; }
out="$(plan windows)"
check '[[ "$out" == *"- linux:"* && "$out" == *"- docker:"* && "$out" == *"- windows:"* && "$out" != *"- macos:"* ]]' "Windows host releases linux, docker and windows by default"
out="$(plan windows --only windows)"
check '[[ "$out" == *"- windows:"* && "$out" != *"- linux:"* && "$out" != *"- docker:"* ]]' "--only windows skips linux and docker"
out="$(plan windows --only=linux)"
check '[[ "$out" == *"- linux:"* && "$out" != *"- windows:"* && "$out" != *"- docker:"* ]]' "--only=linux skips windows and docker"
out="$(plan windows --only docker)"
check '[[ "$out" == *"- docker:"* && "$out" != *"- linux:"* && "$out" != *"- windows:"* ]]' "--only docker releases just the images"
out="$(plan macos --only docker)"; rc=$?
check '[ $rc -ne 0 ] && [[ "$out" == *"from Windows git-bash"* ]]' "Mac host refuses docker"
out="$(plan macos)"
check '[[ "$out" == *"- macos:"* && "$out" != *"- windows:"* ]]' "Mac host releases macos by default"
out="$(plan windows --only macos)"; rc=$?
check '[ $rc -ne 0 ] && [[ "$out" == *"released from a Mac"* ]]' "Windows host refuses macos"
out="$(plan macos --only linux)"; rc=$?
check '[ $rc -ne 0 ] && [[ "$out" == *"from Windows git-bash"* ]]' "Mac host refuses linux"
out="$(plan windows --only windows,ios)"; rc=$?
check '[ $rc -ne 0 ] && [[ "$out" == *"unknown platform"* ]]' "unknown platform refused"

echo "==> 2. update-release-manifest.py"
tmp="$(mktemp -d)"
cat > "$tmp/release.json" <<'EOF'
{
  "windows": {"Version": "agent", "URL": "u", "SHA256": "s"},
  "windows-cli": {"Version": "old", "URL": "u", "SHA256": "s"},
  "macos-cli-arm64": {"Version": "mac-old", "URL": "u", "SHA256": "s"}
}
EOF
python "$REPO/scripts/update-release-manifest.py" "$tmp/release.json" windows-cli v2 https://x/w.zip abc macos-cli-amd64 v3 https://x/m.tgz def
got="$(python -c "import json;d=json.load(open(r'$(cygpath -w "$tmp/release.json")'));print(d['windows-cli']['Version'],d['macos-cli-amd64']['URL'],d['macos-cli-arm64']['Version'],d['windows']['Version'],list(d))")"
want="v2 https://x/m.tgz mac-old agent ['windows', 'windows-cli', 'macos-cli-arm64', 'macos-cli-amd64']"
check '[ "$got" = "$want" ]' "sets given keys, keeps others and their order ($got)"
python "$REPO/scripts/update-release-manifest.py" "$tmp/release.json" windows v u s >/dev/null 2>&1
check '[ $? -ne 0 ]' "refuses LomoAgent's own key"
rm -rf "$tmp"

echo "==> 3. publish_homepage"
sandbox="$(inwsl 'mktemp -d' | tr -d '\r')"
inwsl "
  set -e
  cd '$sandbox'
  git init -q --bare -b master origin.git
  git clone -q origin.git seed 2>/dev/null
  cd seed
  git config user.name seed; git config user.email seed@example.com
  mkdir -p static/windows static/mac migration
  printf '%s\n' '{' '  \"windows-cli\": {\"Version\": \"w1\", \"URL\": \"u\", \"SHA256\": \"s\"},' '  \"macos-cli-arm64\": {\"Version\": \"m1\", \"URL\": \"u\", \"SHA256\": \"s\"}' '}' > static/release.json
  echo ps1 > static/windows/install.ps1; echo sh > static/mac/install.sh; echo '{}' > migration/download-assets.json
  echo readme > README.md
  git add -A; git commit -qm init; git push -q origin master
  cd ..; git clone -q origin.git homepage; git clone -q origin.git other
  cd other; git config user.name other; git config user.email other@example.com
"
origin_json() { inwsl "git -C '$sandbox/origin.git' show master:static/release.json" | tr -d '\r'; }
origin_log() { inwsl "git -C '$sandbox/origin.git' log --format=%s master" | tr -d '\r'; }

publish() {
  (
    set -e
    source "$REPO/scripts/release-all.sh"
    HOST=windows HOMEPAGE_WSL="$sandbox/homepage" REPO_WSL="$REPO_WSL_PATH" COMMIT=abc1234
    MANIFEST_ENTRIES=(windows-cli "$1" "https://example.com/$1.zip" "sha-$1")
    publish_homepage
  ) > "$REPO/.test-release-all.out" 2>&1
}

publish w2; rc=$?
check '[ $rc -eq 0 ]' "plain push succeeds"
check '[[ "$(origin_json)" == *"\"Version\": \"w2\""* ]]' "origin's release.json has windows-cli w2"
check '[ "$(origin_log | head -1)" = "Release windows-cli w2" ]' "commit subject names what was released"

# Another host's release lands first, on another key; homepage also has an unrelated edit.
inwsl "cd '$sandbox/other' && git pull -q && sed -i 's/m1/m2/' static/release.json && git commit -qam 'Release macos-cli-arm64 m2' && git push -q origin master"
inwsl "echo local-edit >> '$sandbox/homepage/README.md'"
publish w3; rc=$?
check '[ $rc -eq 0 ]' "push after origin moved on another key succeeds"
json="$(origin_json)"
check '[[ "$json" == *"\"Version\": \"w3\""* && "$json" == *"\"Version\": \"m2\""* ]]' "origin has both the other release (m2) and this one (w3)"
check '[ "$(origin_log | head -2 | tr "\n" "|")" = "Release windows-cli w3|Release macos-cli-arm64 m2|" ]' "this release was rebased on top of the other"
check '[[ "$(inwsl "cat $sandbox/homepage/README.md" | tr -d "\r")" == *local-edit* ]]' "homepage's unrelated local edit survived the rebase"

# Another release lands first on the same key: a real conflict.
inwsl "cd '$sandbox/other' && git pull -q && sed -i 's/w3/w-other/' static/release.json && git commit -qam 'Release windows-cli w-other' && git push -q origin master"
publish w4; rc=$?
check '[ $rc -ne 0 ]' "conflicting push fails"
check 'grep -q "rebase conflicts" "$REPO/.test-release-all.out"' "says the rebase conflicts"
check '[ "$(origin_log | head -1)" = "Release windows-cli w-other" ]' "origin left as the other release made it"
check '[ "$(inwsl "git -C $sandbox/homepage log -1 --format=%s" | tr -d "\r")" = "Release windows-cli w4" ]' "local commit kept for resolving by hand"
check '! inwsl "test -d $sandbox/homepage/.git/rebase-merge -o -d $sandbox/homepage/.git/rebase-apply"' "no rebase left in progress"

inwsl "rm -rf '$sandbox'"
rm -f "$REPO/.test-release-all.out"

if [ $failures -gt 0 ]; then
  echo "$failures check(s) failed"
  exit 1
fi
echo "All checks passed"
