#!/usr/bin/env bash
# Builds and publishes lomod/lomo-backend release artifacts: Linux (.deb + RPM repos), the
# lomorage Docker images, the Windows CLI installer and the macOS CLI installer. See
# scripts/cross-build-rpi/README.md and installers/{windows,macos}/install.* for why the
# platforms need such different build environments.
#
# Usage: scripts/release-all.sh [--only <platforms>] [--dry-run]
#   --only linux,docker,windows,macos   Comma-separated platforms to release. Default: every
#                                platform this host can build -- linux,docker,windows on Windows,
#                                macos on a Mac. Release only what changed: a Windows-only fix
#                                doesn't need new Linux packages.
#   --dry-run                    Print the plan and exit, without checking or publishing anything.
#
# Where each platform builds:
#   - linux, docker, windows: native Windows git-bash / MSYS (NOT inside WSL). Linux and docker
#     drive WSL2+Docker via `wsl.exe`; Windows builds natively (CGO+vips needs a real Windows
#     host). docker packages the images from the lomo-backend-docker debs the linux step built
#     (releases/LATEST_RELEASE_RPI), via lomo-docker's release.sh.
#   - macos: a Mac (Apple Silicon), building both the arm64 and the amd64 tarball. Signing and
#     notarizing need Apple's tools and the Developer ID certificate in the login keychain.
# RELEASE_HOST=windows|macos overrides host detection (for testing the plan with --dry-run).
#
# One-time prerequisites (not handled by this script):
#   - linux: WSL2 Ubuntu with Docker, the cross-gcc toolchain (see
#     scripts/cross-build-rpi/README.md's "One-time setup"), and the lomorage@gmail.com GPG
#     signing key imported into WSL's ~/.gnupg
#   - docker: the lomo-docker checkout next to this repo, and WSL's docker logged in to Docker
#     Hub as an account that can push lomorage/* (`wsl -e docker login`)
#   - windows: windows-deps/{go,mingw64,pkgconfig-lite-raw,vips} staged locally (see
#     scripts/windows/fetch-vips.ps1)
#   - macos: Go, the Homebrew deps scripts/macos/collect-deps.sh lists (arm64 Homebrew in
#     /opt/homebrew, x86_64 Homebrew in /usr/local for the amd64 tarball), the signing identity
#     in .macos-sign-identity (see the Makefile), and a notarytool keychain profile named in
#     .macos-notary-profile or $MACOS_NOTARY_PROFILE (`xcrun notarytool store-credentials
#     <name>` creates one)
#   - `gh` authenticated with repo access to lomosw/lomosw.github.io -- only its GitHub Releases
#     are still used, to host the Windows/macOS binaries (too big for homepage's git repo). Its
#     website is retired: the installers and release.json are served from homepage
#     (lomorage.com)
#   - the homepage checkout next to this repo, node on PATH and `npm ci` run once in it, for
#     scripts/sync-installers-to-homepage.sh
#   - WSL's ~/.ssh/known_hosts already has github.com's host key (run
#     `ssh-keyscan -t ed25519,rsa github.com >> ~/.ssh/known_hosts` once if a `git push` from WSL
#     ever hangs with no output)
#
# Does NOT touch lomo-backend's own git history: every artifact is built from HEAD, which must
# be pushed and have passed CI (LATEST_RELEASE embeds `git rev-parse --short=7 HEAD`).
#
# The last step commits release.json (and the synced installers) in homepage and pushes. If
# homepage/master moved meanwhile -- typically the other host's release -- it rebases onto it
# and pushes again; it stops on a real conflict and leaves the commit for you to resolve.
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HOMEPAGE="${HOMEPAGE_DIR:-$REPO/../homepage}"
GH_RELEASES_REPO="lomosw/lomosw.github.io"
# The same checkouts as seen from WSL (Windows host only).
REPO_WSL="${REPO_WSL:-/mnt/c/Users/fuji2/Work/lomo-backend}"
LOMOWARE_SITE_WSL="${LOMOWARE_SITE_WSL:-/mnt/c/Users/fuji2/Work/lomoware.github.io}"
HOMEPAGE_WSL="${HOMEPAGE_WSL:-/mnt/c/Users/fuji2/Work/homepage}"
LOMO_DOCKER_WSL="${LOMO_DOCKER_WSL:-/mnt/c/Users/fuji2/Work/lomo-docker}"

log() { echo -e "\n=== $1 ===\n"; }
die() { echo "$1" >&2; exit 1; }

# Every WSL step below uses `wsl -e bash -lc "<script>"`, never `wsl -- bash -lc`: with `--`,
# wsl.exe runs the command line through WSL's default shell first, which expands each \$VAR in
# <script> before the inner bash ever assigns it (e.g. `X=1; echo \$X` prints an empty line).
# -e hands the arguments to bash unchanged.

# Sets HOST and DO_LINUX/DO_WINDOWS/DO_MACOS from the arguments, and prints the plan.
parse_args() {
  ONLY=""
  DRY_RUN=0
  while [ $# -gt 0 ]; do
    case "$1" in
      --only) [ $# -ge 2 ] || die "--only needs a value"; ONLY="$2"; shift 2 ;;
      --only=*) ONLY="${1#--only=}"; shift ;;
      --dry-run) DRY_RUN=1; shift ;;
      *) die "unknown argument: $1 (usage: release-all.sh [--only linux,windows,macos] [--dry-run])" ;;
    esac
  done

  HOST="${RELEASE_HOST:-}"
  if [ -z "$HOST" ]; then
    case "$(uname -s)" in
      MINGW*|MSYS*|CYGWIN*) HOST=windows ;;
      Darwin) HOST=macos ;;
      *) die "unsupported host $(uname -s): run this from Windows git-bash (linux, windows) or a Mac (macos)" ;;
    esac
  fi
  case "$HOST" in
    windows) DEFAULT_PLATFORMS="linux,docker,windows" ;;
    macos) DEFAULT_PLATFORMS="macos" ;;
    *) die "RELEASE_HOST must be windows or macos, not $HOST" ;;
  esac

  DO_LINUX=0 DO_DOCKER=0 DO_WINDOWS=0 DO_MACOS=0
  IFS=',' read -r -a PLATFORMS <<< "${ONLY:-$DEFAULT_PLATFORMS}"
  for p in "${PLATFORMS[@]}"; do
    case "$p" in
      linux|docker|windows) [ "$HOST" = windows ] || die "$p is released from Windows git-bash, not from a Mac" ;;
      macos) [ "$HOST" = macos ] || die "macos is released from a Mac, not from Windows" ;;
      *) die "unknown platform '$p' (linux, docker, windows, macos)" ;;
    esac
    case "$p" in
      linux) DO_LINUX=1 ;;
      docker) DO_DOCKER=1 ;;
      windows) DO_WINDOWS=1 ;;
      macos) DO_MACOS=1 ;;
    esac
  done

  PLAN=()
  [ $DO_LINUX = 1 ] && PLAN+=("linux: build .deb/.rpm in WSL2+Docker, sign and push the apt/rpm repos (lomoware.github.io)")
  [ $DO_DOCKER = 1 ] && PLAN+=("docker: build + smoke-test lomorage/{amd64,arm64,raspberrypi}-lomorage from the lomo-backend-docker debs, push to Docker Hub")
  [ $DO_WINDOWS = 1 ] && PLAN+=("windows: make release-windows, GitHub Release on $GH_RELEASES_REPO, release.json windows-cli")
  [ $DO_MACOS = 1 ] && PLAN+=("macos: make release-macos + release-macos-amd64, notarize, GitHub Release on $GH_RELEASES_REPO, release.json macos-cli-arm64/amd64")
  log "Plan ($HOST host)"
  printf '  - %s\n' "${PLAN[@]}"
  if [ $DRY_RUN = 1 ]; then
    echo -e "\nDry run: nothing checked or published."
    exit 0
  fi
}

# release.json entries to write in the last step: key, version, url, sha256 (repeated).
MANIFEST_ENTRIES=()
SUMMARY=()

# --- Preflight ------------------------------------------------------------------------------

preflight() {
  log "Preflight checks"
  command -v gh >/dev/null || die "gh CLI not found on PATH"
  gh auth status >/dev/null 2>&1 || die "gh is not authenticated (run: gh auth login)"
  if [ "$HOST" = windows ]; then
    command -v wsl >/dev/null || die "wsl.exe not found"
  fi
  if [ $DO_WINDOWS = 1 ]; then
    [ -x "$REPO/windows-deps/mingw64/bin/mingw32-make.exe" ] || die "windows-deps/mingw64 not staged -- see this script's header"
  fi
  if [ $DO_LINUX = 1 ] || [ $DO_DOCKER = 1 ]; then
    wsl -d Ubuntu -e bash -lc "docker info" >/dev/null 2>&1 || die "Docker isn't reachable from WSL Ubuntu"
  fi
  if [ $DO_DOCKER = 1 ]; then
    wsl -d Ubuntu -e bash -lc "test -x '$LOMO_DOCKER_WSL/release.sh'" || die "lomo-docker checkout with release.sh not found at $LOMO_DOCKER_WSL"
    wsl -d Ubuntu -e bash -lc "docker info 2>/dev/null | grep -q '^ *Username:'" \
      || die "WSL's docker isn't logged in to Docker Hub (run: wsl -e docker login)"
  fi
  if [ $DO_MACOS = 1 ]; then
    MACOS_SIGN_IDENTITY="${MACOS_SIGN_IDENTITY:-$(cat "$REPO/.macos-sign-identity" 2>/dev/null || true)}"
    [ -n "$MACOS_SIGN_IDENTITY" ] || die "no signing identity: ad-hoc signed builds fail notarization. Set .macos-sign-identity (see the Makefile)"
    MACOS_NOTARY_PROFILE="${MACOS_NOTARY_PROFILE:-$(cat "$REPO/.macos-notary-profile" 2>/dev/null || true)}"
    [ -n "$MACOS_NOTARY_PROFILE" ] || die "no notarytool keychain profile: put its name in .macos-notary-profile (xcrun notarytool store-credentials <name> creates one)"
    xcrun notarytool history --keychain-profile "$MACOS_NOTARY_PROFILE" >/dev/null 2>&1 \
      || die "notarytool can't use keychain profile '$MACOS_NOTARY_PROFILE'"
  fi
  # Only release a commit CI (.github/workflows/ci.yml) has passed. Every artifact is built from
  # the working tree, so it must also match that commit exactly. SKIP_CI_CHECK=1 overrides, for
  # emergencies.
  HEAD_SHA="$(git -C "$REPO" rev-parse HEAD)"
  if [ "${SKIP_CI_CHECK:-}" = 1 ]; then
    echo "!! SKIP_CI_CHECK=1: releasing $HEAD_SHA without checking CI" >&2
  else
    git -C "$REPO" diff --quiet HEAD -- || die "Tracked files have uncommitted changes; commit and push them so CI can test what gets released"
    git -C "$REPO" fetch -q origin
    [ -n "$(git -C "$REPO" branch -r --contains "$HEAD_SHA")" ] || die "HEAD $HEAD_SHA isn't pushed, so CI hasn't run on it; push it and wait for CI"
    CI_STATE="$(gh run list --repo lomorage/lomod --workflow ci.yml --commit "$HEAD_SHA" \
      --json status,conclusion --jq 'map("\(.status)/\(.conclusion)") | if length == 0 then "no run" elif any(. == "completed/success") then "passed" else .[0] end')"
    [ "$CI_STATE" = passed ] || die "CI for $HEAD_SHA: $CI_STATE (see https://github.com/lomorage/lomod/actions). Not releasing."
    echo "CI passed for $HEAD_SHA"
  fi
  COMMIT="$(git -C "$REPO" rev-parse --short=7 HEAD)"
  # Copies installers/{windows,macos} into homepage and runs its tests now, before anything is
  # published, so a failure stops the release here; the last step commits the result with
  # release.json.
  bash "$REPO/scripts/sync-installers-to-homepage.sh"
}

# --- Linux ------------------------------------------------------------------------------------

release_linux() {
  log "Linux build (WSL2 + Docker): arm64/armhf/amd64 .deb, jammy/noble lomo-vips, Fedora/Rocky9/openSUSE RPMs"
  wsl -d Ubuntu -e bash -lc "set -e; cd '$REPO_WSL' && bash scripts/cross-build-rpi/build-all.sh"

  log "Linux publish: sign + push apt (buster/bullseye/bookworm/trixie/jammy/noble) and rpm (fedora/rocky9/opensuse) repos"
  wsl -d Ubuntu -e bash -lc "
    set -e
    cd '$REPO_WSL'
    bash scripts/cross-build-rpi/publish-debian.sh
    bash scripts/cross-build-rpi/rpm/publish-fedora.sh
    bash scripts/cross-build-rpi/rpm/publish-rocky9.sh
    bash scripts/cross-build-rpi/rpm/publish-opensuse.sh

    cd '$LOMOWARE_SITE_WSL'
    git add debian/buster debian/bullseye debian/bookworm debian/trixie debian/jammy debian/noble rpm/fedora rpm/rocky9 rpm/opensuse
    if git diff --cached --quiet; then
      echo 'lomoware.github.io: nothing new to publish'
    else
      LOMOD_VERSION=\$(cat '$REPO_WSL/releases/LATEST_RELEASE_RPI')
      git -c user.name='Jeromy Fu' -c user.email='fuji246@gmail.com' commit -m \"Publish lomod/lomo-backend \$LOMOD_VERSION\"
      git push origin master
    fi
  "
  SUMMARY+=("linux: $(tr -d '\r' < "$REPO/releases/LATEST_RELEASE_RPI") (apt/rpm repos)")
}

# --- Docker -----------------------------------------------------------------------------------

# Images for the release the linux step just built (or, with --only docker, the last one built:
# releases/LATEST_RELEASE_RPI). Built from the local lomo-backend-docker debs rather than the apt
# repo, so this doesn't wait for GitHub Pages + the CDN to pick up the publish.
release_docker() {
  log "Docker images: build, smoke-test and push lomorage/{amd64,arm64,raspberrypi}-lomorage"
  wsl -d Ubuntu -e bash -lc "
    set -e
    cd '$REPO_WSL'
    V=\$(tr -d '\r' < releases/LATEST_RELEASE_RPI)
    ls releases/lomo-backend-docker_\${V}_*.deb >/dev/null 2>&1 || bash scripts/cross-build-rpi/package-docker.sh \"\$V\"
    bash '$LOMO_DOCKER_WSL/release.sh' '$REPO_WSL/releases' \"\$V\"
  "
  SUMMARY+=("docker: $(tr -d '\r' < "$REPO/releases/LATEST_RELEASE_RPI") (lomorage/*-lomorage:latest on Docker Hub)")
}

# --- Windows ----------------------------------------------------------------------------------

release_windows() {
  log "Windows build: lomod.exe + lomoupg.exe + runtime deps"
  (
    export GOROOT="$REPO/windows-deps/go"
    export PATH="$REPO/windows-deps/go/bin:$REPO/windows-deps/mingw64/bin:$REPO/windows-deps/pkgconfig-lite-raw/pkg-config-lite-0.28-1/bin:$HOME/go/bin:$PATH"
    "$REPO/windows-deps/mingw64/bin/mingw32-make.exe" -C "$REPO" release-windows \
      WINDOWS_MINGW_BIN="$REPO/windows-deps/mingw64/bin" \
      WINDOWS_PKGCONFIG_BIN="$REPO/windows-deps/pkgconfig-lite-raw/pkg-config-lite-0.28-1/bin" \
      WINDOWS_VIPS_DIR="$REPO/windows-deps/vips"
  )

  local version zip sha256 ffmpeg_build url
  version="$(cat "$REPO/LATEST_RELEASE")"
  zip="lomorage-windows-amd64-${version}.zip"
  sha256="$(cut -d' ' -f1 "$REPO/releases/windows/${zip}.sha256")"
  # Written by scripts/windows/collect-deps.ps1: ffmpeg comes from a rolling upstream tag, so
  # the release notes are the record of which build this zip bundles.
  ffmpeg_build="$(tr -d '\r' < "$REPO/dist-windows/ffmpeg-build.txt")"

  log "Windows publish: GitHub Release on $GH_RELEASES_REPO"
  gh release create "lomod-windows-cli-${version}" \
    "$REPO/releases/windows/${zip}" \
    --repo "$GH_RELEASES_REPO" \
    --title "lomod Windows CLI installer ${version}" \
    --notes "lomod.exe + lomoupg.exe + vips/exiftool/ffmpeg runtime deps for the headless Windows CLI installer (installers/windows/install.ps1 in lomo-backend). Built from lomo-backend@${COMMIT}.

Bundled ffmpeg:
\`\`\`
${ffmpeg_build}
\`\`\`" \
    --latest

  url="https://github.com/$GH_RELEASES_REPO/releases/download/lomod-windows-cli-${version}/${zip}"
  MANIFEST_ENTRIES+=(windows-cli "$version" "$url" "$sha256")
  SUMMARY+=("windows: $version ($zip)")
}

# --- macOS ------------------------------------------------------------------------------------

# Submits one built tarball's contents to Apple and waits for the verdict. Nothing is stapled:
# the tarball ships bare Mach-O binaries, which can't carry a ticket, so Gatekeeper looks the
# notarization up online -- the same as every macOS release so far.
notarize_macos() {
  local dist="$1" zip
  zip="$(mktemp -d)/$(basename "$dist").zip"
  ditto -c -k --keepParent "$dist" "$zip"
  local out
  out="$(xcrun notarytool submit "$zip" --keychain-profile "$MACOS_NOTARY_PROFILE" --wait 2>&1)" || true
  echo "$out"
  rm -f "$zip"
  echo "$out" | grep -q "status: Accepted" || die "notarization of $dist was not accepted (see above; xcrun notarytool log <id> --keychain-profile $MACOS_NOTARY_PROFILE shows why)"
}

release_macos() {
  local arch version tarball tag notes
  local -a assets=() versions=()
  for arch in arm64 amd64; do
    log "macOS build: $arch"
    local target=release-macos dist="$REPO/dist-macos"
    if [ "$arch" = amd64 ]; then target=release-macos-amd64; dist="$REPO/dist-macos-amd64"; fi
    make -C "$REPO" "$target" MACOS_SIGN_IDENTITY="$MACOS_SIGN_IDENTITY"
    version="$(cat "$REPO/LATEST_RELEASE")"
    tarball="lomorage-macos-${arch}-${version}.tar.gz"

    log "macOS notarize: $arch"
    notarize_macos "$dist"

    assets+=("$REPO/releases/macos/$tarball" "$REPO/releases/macos/$tarball.sha256")
    versions+=("$arch $version")
    eval "MACOS_${arch}_VERSION=\$version MACOS_${arch}_TARBALL=\$tarball"
  done

  # One release for both tarballs, tagged with the arm64 build's version.
  tag="lomod-macos-cli-${MACOS_arm64_VERSION}"
  log "macOS publish: GitHub Release on $GH_RELEASES_REPO"
  notes="lomod backend for macOS: arm64 (Apple Silicon) and amd64 (Intel) tarballs, each with a self-contained bundle of vips/ffmpeg/exiftool dylibs so Homebrew is not required. Signed with a Developer ID Application certificate (hardened runtime, secure timestamp) and notarized by Apple (notarytool: Accepted).

Built from lomo-backend@${COMMIT}."
  gh release create "$tag" "${assets[@]}" --repo "$GH_RELEASES_REPO" \
    --title "lomod macOS CLI ${MACOS_arm64_VERSION}" --notes "$notes" --latest=false

  for arch in arm64 amd64; do
    eval "version=\$MACOS_${arch}_VERSION tarball=\$MACOS_${arch}_TARBALL"
    MANIFEST_ENTRIES+=("macos-cli-$arch" "$version" \
      "https://github.com/$GH_RELEASES_REPO/releases/download/$tag/$tarball" \
      "$(cut -d' ' -f1 "$REPO/releases/macos/$tarball.sha256")")
  done
  SUMMARY+=("macos: ${versions[*]}")
}

# --- homepage ---------------------------------------------------------------------------------

publish_homepage() {
  [ ${#MANIFEST_ENTRIES[@]} -gt 0 ] || return 0
  local keys=() i
  for ((i = 0; i < ${#MANIFEST_ENTRIES[@]}; i += 4)); do keys+=("${MANIFEST_ENTRIES[i]} ${MANIFEST_ENTRIES[i+1]}"); done
  log "Update homepage/static/release.json ($(IFS=,; echo "${keys[*]}" | sed 's/ [^,]*//g')), commit it with the synced installers, and push"

  # From WSL on Windows, where git's SSH setup for pushing lives; directly on a Mac.
  local hp repo
  if [ "$HOST" = windows ]; then hp="$HOMEPAGE_WSL"; repo="$REPO_WSL"; else hp="$HOMEPAGE"; repo="$REPO"; fi
  local args="" e
  for e in "${MANIFEST_ENTRIES[@]}"; do args+=" '$e'"; done
  local subject
  subject="Release $(IFS=';'; echo "${keys[*]}" | sed 's/;/, /g')"

  local script="
    set -e
    cd '$hp'
    python3 '$repo/scripts/update-release-manifest.py' static/release.json $args
    git config user.name >/dev/null 2>&1 || git config user.name 'Jeromy Fu'
    git config user.email >/dev/null 2>&1 || git config user.email 'fuji246@gmail.com'
    FILES='static/release.json static/windows/install.ps1 static/mac/install.sh migration/download-assets.json'
    # if/else rather than an early 'exit 0': in WSL's login shell (-l) the exit builtin runs
    # Ubuntu's ~/.bash_logout, whose clear_console fails without a terminal and turns it into 1.
    if git diff --quiet HEAD -- \$FILES; then
      echo 'homepage already up to date, nothing to publish'
    else
      git commit -m \"$subject\" \
        -m \"Installers synced from lomo-backend@$COMMIT by scripts/sync-installers-to-homepage.sh.\" \
        -- \$FILES
      if ! git push origin master; then
        # Usually the other host's release landed meanwhile. Its commit touches other keys,
        # so a rebase applies cleanly; --autostash parks homepage's unrelated local edits.
        echo 'push rejected, rebasing onto origin/master'
        if git pull --rebase --autostash origin master; then
          git push origin master
        else
          git rebase --abort || true
          echo '' >&2
          echo '!!! homepage/master moved and the rebase conflicts. Not auto-resolving.' >&2
          echo '!!! Your commit is safe on the local master branch. To resolve by hand:' >&2
          echo '!!!   cd $hp && git log --oneline HEAD..origin/master' >&2
          echo '!!!   then merge/rebase -- if a conflicting file also has unrelated local edits,' >&2
          echo '!!!   check with \"diff --strip-trailing-cr\" whether that side is real content or' >&2
          echo '!!!   just CRLF noise before deciding how to resolve it.' >&2
          false
        fi
      fi
    fi
  "
  if [ "$HOST" = windows ]; then
    wsl -d Ubuntu -e bash -lc "$script"
  else
    bash -c "$script"
  fi
}

main() {
  parse_args "$@"
  preflight
  if [ $DO_LINUX = 1 ]; then release_linux; fi
  if [ $DO_DOCKER = 1 ]; then release_docker; fi
  if [ $DO_WINDOWS = 1 ]; then release_windows; fi
  if [ $DO_MACOS = 1 ]; then release_macos; fi
  publish_homepage
  log "Done"
  printf '  - %s\n' "${SUMMARY[@]}"
}

# When sourced (by scripts/test-release-all.sh), only define the functions.
if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  main "$@"
fi
