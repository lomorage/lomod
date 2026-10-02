#!/usr/bin/env bash
# Smoke test for a freshly built lomod: start it against an empty data dir (a first run, so no
# account exists yet) and check what a new user hits first -- the process stays up, /status
# answers, /welcome serves the setup page, and its QR code decodes to the lomorage.com/s/ link
# the phone camera and LomoMobile expect (see handler/setup.go).
#
# Usage: scripts/ci/smoke-setup-qr.sh path/to/lomod
# Needs curl and zbarimg (apt: zbar-tools). Run by .github/workflows/ci.yml.
set -euo pipefail

LOMOD="$(cd "$(dirname "${1:?usage: $0 path/to/lomod}")" && pwd)/$(basename "$1")"
PORT="${PORT:-8123}"
URL="http://127.0.0.1:$PORT"
WORK="$(mktemp -d)"
LOG="$WORK/lomod.log"

fail() {
  echo "SMOKE FAIL: $*" >&2
  if ! kill -0 "$PID" 2>/dev/null; then
    wait "$PID" 2>/dev/null; echo "lomod exit status: $?" >&2
  fi
  echo "--- lomod stdout/stderr (last 40 lines) ---" >&2
  tail -n 40 "$LOG" >&2 || true
  find "$WORK/base" -name '*.log' 2>/dev/null | while read -r f; do
    echo "--- $f (last 40 lines) ---" >&2
    tail -n 40 "$f" >&2
  done
  exit 1
}

# No --exe-dir: like a Linux package install, lomod finds exiftool/ffmpeg next to itself or on
# PATH. (With --exe-dir it only looks in that directory.)
"$LOMOD" --base "$WORK/base" --no-mdns --port "$PORT" >"$LOG" 2>&1 &
PID=$!
trap 'kill "$PID" 2>/dev/null || true; wait "$PID" 2>/dev/null || true; rm -rf "$WORK"' EXIT

for _ in $(seq 1 60); do
  kill -0 "$PID" 2>/dev/null || fail "lomod exited during startup"
  curl -fs -o /dev/null "$URL/status" && break
  sleep 1
done
curl -fs -o /dev/null "$URL/status" || fail "/status did not answer within 60s"
echo "ok: lomod is up and /status answers"

page="$(curl -fs "$URL/welcome")" || fail "GET /welcome failed"
grep -q 'id="welcome-scan-section"' <<<"$page" || fail "/welcome is not the first-run setup page"
grep -q 'src="/welcome/qrcode.png"' <<<"$page" || fail "/welcome has no setup QR code"
echo "ok: /welcome serves the first-run setup page"

curl -fs -o "$WORK/qr.png" "$URL/welcome/qrcode.png" || fail "GET /welcome/qrcode.png failed"
link="$(zbarimg --quiet --raw "$WORK/qr.png")" || fail "could not decode the setup QR code"
echo "setup QR code: $link"
[[ "$link" == https://lomorage.com/s/#* ]] || fail "QR code is not a https://lomorage.com/s/# link"
[[ "$link" =~ [#\&]server=[^\&]+(%3A|:)$PORT(\&|$) ]] || fail "QR code's server= does not point at port $PORT"
[[ "$link" =~ [#\&]uuid=[^\&]+ ]] || fail "QR code has no uuid="
echo "ok: setup QR code is a lomorage.com/s/ link to this server"

kill -0 "$PID" 2>/dev/null || fail "lomod died while serving the checks"
echo "SMOKE PASS"
