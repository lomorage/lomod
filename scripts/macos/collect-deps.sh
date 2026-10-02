#!/usr/bin/env bash
#
# Stages runtime dependencies for a lomod macOS release bundle into --dest-dir:
#   - the libvips dylib tree lomod is linked against (via Homebrew), bundled next to the
#     binaries and rewritten to load from @executable_path/libs (so the release doesn't
#     require Homebrew/vips to be installed on the end user's machine)
#   - ffmpeg/ffprobe from Homebrew, dylib-bundled the same way
#   - exiftool from Homebrew, copied wholesale (it's a perl script + module tree, not a
#     dylib consumer -- macOS ships /usr/bin/perl, so no bundling needed there)
#
# Unlike Windows (fetch-vips.ps1 stages a prebuilt, self-contained vips-dev zip), there is no
# equivalent portable vips distribution for macOS, so this script bundles whatever libvips
# build/version is currently installed via Homebrew on the build host -- run
# `brew install vips ffmpeg exiftool dylibbundler` first (build-lomod-mac already requires
# vips to build at all; this just adds the other three).
#
# Requires dylibbundler (`brew install dylibbundler`): it recursively resolves each binary's
# non-system dylib dependencies, copies them into --dest-dir/libs, and rewrites the binary's
# and every copied dylib's load commands to reference each other via @executable_path/libs --
# so the result runs standalone without Homebrew on the target machine.
#
# Codesigning: dylibbundler's own ad-hoc signing (`-ns` disables it below) runs before the
# LC_RPATH dedup pass, which would invalidate it anyway via install_name_tool -- so this script
# does a single real signing pass at the very end instead, over every Mach-O file it touched
# (the main binary, ffmpeg, ffprobe, and everything under libs/). With --sign-identity unset,
# that pass ad-hoc signs (`--sign -`), matching dylibbundler's old default behavior. With
# --sign-identity set to a "Developer ID Application: ..." identity from the keychain, it signs
# for real with the hardened runtime + a secure timestamp (both required for notarization) --
# critically, every bundled dylib ends up signed under the same Team ID as the main binary,
# which is required for the hardened runtime's Library Validation to allow loading them at all.
set -euo pipefail

DEST_DIR=""
BINARY=""
SIGN_IDENTITY=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --dest-dir) DEST_DIR="$2"; shift 2 ;;
        --binary) BINARY="$2"; shift 2 ;;
        --sign-identity) SIGN_IDENTITY="$2"; shift 2 ;;
        *) echo "unknown argument: $1" >&2; exit 1 ;;
    esac
done

if [[ -z "$DEST_DIR" || -z "$BINARY" ]]; then
    echo "usage: $0 --dest-dir <dir> --binary <path to lomod binary> [--sign-identity <codesign identity>]" >&2
    exit 1
fi

if ! command -v dylibbundler >/dev/null 2>&1; then
    echo "dylibbundler not found -- install it with: brew install dylibbundler" >&2
    exit 1
fi
if ! command -v brew >/dev/null 2>&1; then
    echo "Homebrew not found -- required to locate ffmpeg/exiftool for bundling" >&2
    exit 1
fi

LIBS_DIR="${DEST_DIR}/libs"

# -od (--overwrite-dir) *deletes and recreates* the whole dest dir on every invocation, so it
# can only be passed on the first of these several dylibbundler calls that all share LIBS_DIR --
# otherwise each subsequent call wipes out the previous binaries' already-bundled libs. Later
# calls use -cd (create if missing, doesn't touch existing contents) + -of (ok to overwrite an
# individual file, for libs multiple binaries here both depend on, e.g. libSystem-adjacent libs).
echo "==> Bundling dylibs for lomod"
dylibbundler -od -of -ns -b \
    -x "${BINARY}" \
    -d "${LIBS_DIR}" \
    -p "@executable_path/libs/"

# ffprobe ships inside the ffmpeg formula (there's no separate "ffprobe" formula), so both
# come from a single `brew --prefix ffmpeg` lookup.
ffmpeg_prefix="$(brew --prefix ffmpeg 2>/dev/null || true)"
for tool in ffmpeg ffprobe; do
    if [[ -z "${ffmpeg_prefix}" || ! -x "${ffmpeg_prefix}/bin/${tool}" ]]; then
        echo "==> Skipping ${tool}: not found under 'brew --prefix ffmpeg' (install with: brew install ffmpeg)"
        continue
    fi
    echo "==> Bundling dylibs for ${tool}"
    cp "${ffmpeg_prefix}/bin/${tool}" "${DEST_DIR}/${tool}"
    chmod +w "${DEST_DIR}/${tool}"
    dylibbundler -cd -of -ns -b \
        -x "${DEST_DIR}/${tool}" \
        -d "${LIBS_DIR}" \
        -p "@executable_path/libs/"
done

exiftool_version="$(brew list --versions exiftool 2>/dev/null | awk '{print $2}')"
if [[ -n "${exiftool_version}" ]]; then
    echo "==> Copying exiftool"
    exiftool_cellar="$(brew --cellar exiftool)/${exiftool_version}"
    # Copy the whole Cellar tree rather than assuming a specific internal layout (it varies
    # between a flat prefix and a libexec/{bin,lib} split across Homebrew versions of the
    # formula), then symlink whichever copied file is the actual exiftool script.
    cp -R "${exiftool_cellar}" "${DEST_DIR}/exiftool-pkg"
    real_bin="$(find "${DEST_DIR}/exiftool-pkg" -type f -name exiftool -perm -u+x | head -1)"
    if [[ -n "${real_bin}" ]]; then
        ln -sf "${real_bin#"${DEST_DIR}"/}" "${DEST_DIR}/exiftool"
    else
        echo "!! Copied exiftool-pkg but couldn't locate the exiftool executable inside it -- check ${DEST_DIR}/exiftool-pkg manually" >&2
    fi
else
    echo "==> Skipping exiftool: not installed via Homebrew (install with: brew install exiftool)"
fi

# dylibbundler's recursive dependency walk sometimes "fixes" (and install_name_tool
# -add_rpath's) the same dylib twice when it's reachable via more than one path in the
# dependency graph -- observed with vips's OpenEXR dependency, which pulls in both
# libOpenEXR and libOpenEXRCore, each independently depending on Imath, etc. A file with two
# identical LC_RPATH entries fails dyld's "duplicate LC_RPATH" validation at load time, so
# collapse any duplicates down to one occurrence per unique path.
dedupe_rpaths() {
    local file="$1" rpath="@executable_path/libs/" count
    count="$(otool -l "${file}" 2>/dev/null | awk -v p="${rpath}" '$1=="path" && $2==p { c++ } END { print c+0 }')"
    while [[ "${count}" -gt 1 ]]; do
        install_name_tool -delete_rpath "${rpath}" "${file}"
        count="$(otool -l "${file}" 2>/dev/null | awk -v p="${rpath}" '$1=="path" && $2==p { c++ } END { print c+0 }')"
    done
}

echo "==> De-duplicating any repeated LC_RPATH entries"
for f in "${BINARY}" "${DEST_DIR}/ffmpeg" "${DEST_DIR}/ffprobe" "${LIBS_DIR}"/*; do
    [[ -f "${f}" ]] || continue
    dedupe_rpaths "${f}"
done

if [[ -n "${SIGN_IDENTITY}" ]]; then
    echo "==> Codesigning with ${SIGN_IDENTITY} (hardened runtime + secure timestamp)"
else
    echo "==> Codesigning ad-hoc (no --sign-identity given)"
fi
# exiftool-pkg is mostly pure perl (no bundling needed, see above), but some of its XS modules
# (e.g. IO::Compress::Brotli) ship a compiled Mach-O .bundle -- notarization scans and rejects
# every unsigned/ad-hoc Mach-O in the archive regardless of whether it's ever executed, so these
# need the same real-identity signing pass even though they're not part of lomod's own bundling.
EXIFTOOL_MACHO=()
if [[ -d "${DEST_DIR}/exiftool-pkg" ]]; then
    while IFS= read -r -d '' f; do
        EXIFTOOL_MACHO+=("${f}")
    done < <(find "${DEST_DIR}/exiftool-pkg" -type f -perm -u+x -print0 | xargs -0 file | grep "Mach-O" | cut -d: -f1 | tr '\n' '\0')
fi
for f in "${BINARY}" "${DEST_DIR}/ffmpeg" "${DEST_DIR}/ffprobe" "${LIBS_DIR}"/* "${EXIFTOOL_MACHO[@]}"; do
    [[ -f "${f}" ]] || continue
    if [[ -n "${SIGN_IDENTITY}" ]]; then
        codesign --force --options runtime --timestamp --sign "${SIGN_IDENTITY}" "${f}"
    else
        codesign --force --sign - "${f}"
    fi
done

echo "==> Done staging macOS dependencies into ${DEST_DIR}"
