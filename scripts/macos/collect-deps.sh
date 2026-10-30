#!/usr/bin/env bash
#
# Stages runtime dependencies for a lomod macOS release bundle into --dest-dir:
#   - the libvips dylib tree lomod is linked against (via Homebrew), bundled next to the
#     binaries and rewritten to load from @executable_path/libs (so the release doesn't
#     require Homebrew/vips to be installed on the end user's machine)
#   - ffmpeg/ffprobe from Homebrew, dylib-bundled the same way
#   - exiftool from Homebrew, copied wholesale (it's a perl script + module tree, not a
#     dylib consumer -- macOS ships /usr/bin/perl, so no bundling needed there)
#   - libvips's own dynamically loaded modules (heif, jxl, ...), which nothing links, so
#     dylibbundler cannot find them, together with the libraries they pull in
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
# so the result runs standalone without Homebrew on the target machine. A dlopen'd module is
# not a link-time dependency and so is invisible to it -- see the vips module staging below.
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

# `|| true` because of pipefail: with exiftool not installed, brew exits non-zero and that,
# not awk's 0, becomes the pipeline's status -- which under set -e killed the script here
# rather than letting it reach the "Skipping exiftool" branch below.
exiftool_version="$(brew list --versions exiftool 2>/dev/null | awk '{print $2}' || true)"
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

# libvips 8.13+ can build some of its loaders -- heif (HEIC/AVIF), jxl, magick, openslide -- as
# dynamically loaded modules instead of linking them into libvips itself, and Homebrew's vips is
# built that way. dylibbundler only follows link-time dependencies, so it never sees them: until
# this was added the bundle shipped a libvips with no heifload/jxlload/magickload operation at
# all, which left HEIF/AVIF previews silently going through ffmpeg and JPEG XL failing outright
# (lomorage/lomod#20). Copy the module directory in and bundle each module's own dependencies --
# libheif, and under it libde265 for HEVC and libaom/libdav1d for AV1, plus libjxl and
# libMagickCore. @executable_path resolves against the main executable no matter which dylib
# triggered the load, so the modules can use the same prefix as everything else here.
#
# The destination is lib/, not libs/: vips_init looks for modules in
# <libdir>/vips-modules-<major>.<minor> and derives libdir as $VIPSHOME/lib, so that name is
# libvips's to pick, not ours. common/preview/vips_modules_darwin.go sets VIPSHOME to the bundle
# root at startup -- see its comment for why none of libvips's other search paths can find them.
vips_prefix="$(brew --prefix vips 2>/dev/null || true)"
modules_src=""
if [[ -n "${vips_prefix}" ]]; then
    modules_src="$(find "${vips_prefix}/lib" -maxdepth 1 -type d -name 'vips-modules-*' 2>/dev/null | head -1 || true)"
fi
MODULES_DIR=""
if [[ -n "${modules_src}" ]]; then
    MODULES_DIR="${DEST_DIR}/lib/$(basename "${modules_src}")"
    echo "==> Staging vips modules from ${modules_src}"
    rm -rf "${MODULES_DIR}"
    mkdir -p "${MODULES_DIR}"
    # Copy whatever is in there rather than globbing an extension: glib's module suffix is "so"
    # on macOS, not "dylib", and that is glib's choice to change.
    cp -R "${modules_src}/." "${MODULES_DIR}/"
    find "${MODULES_DIR}" -type f -exec chmod +w {} +
    for m in "${MODULES_DIR}"/*; do
        [[ -f "${m}" ]] || continue
        echo "==> Bundling dylibs for $(basename "${m}")"
        dylibbundler -cd -of -ns -b \
            -x "${m}" \
            -d "${LIBS_DIR}" \
            -p "@executable_path/libs/"
    done
else
    # A non-modular libvips (modules disabled at build time, or vips older than 8.13) links
    # these loaders in directly, so there is nothing to stage -- the check at the end of this
    # script has to pass either way.
    echo "==> No vips-modules-* directory under the vips prefix -- assuming a non-modular libvips"
fi

# A copy of Homebrew's vips CLI, bundled the same way, is the only thing that can ask the
# *bundled* libvips what it can decode: run from anywhere outside DEST_DIR it would load
# Homebrew's own libvips through its absolute install names and report on the build host rather
# than on the artifact. It sits at the bundle root so @executable_path/libs/ resolves, and is
# deleted again after the check below, so the shipped tarball holds exactly the files it used to
# plus the modules. Its dependencies are a subset of libvips's, so it adds nothing to libs/.
VIPS_PROBE=""
if [[ -n "${vips_prefix}" && -x "${vips_prefix}/bin/vips" ]]; then
    VIPS_PROBE="${DEST_DIR}/vips-probe"
    cp "${vips_prefix}/bin/vips" "${VIPS_PROBE}"
    chmod +w "${VIPS_PROBE}"
    echo "==> Bundling dylibs for the vips decode probe"
    dylibbundler -cd -of -ns -b \
        -x "${VIPS_PROBE}" \
        -d "${LIBS_DIR}" \
        -p "@executable_path/libs/"
fi

# Clear the probe out however this script exits: a failed check must not leave an extra
# executable behind for the Makefile's tar to pick up.
PROBE_TMP="$(mktemp -d)"
trap 'rm -rf "${PROBE_TMP}"; [[ -z "${VIPS_PROBE}" ]] || rm -f "${VIPS_PROBE}"' EXIT

# The modules are dlopen'd rather than linked, so the dedup and signing passes below have to
# cover them as well -- a duplicate LC_RPATH or a missing signature stops a module loading just
# as it would a dylib, except the failure surfaces as a missing operation instead of a dyld
# error. The probe needs both too, since install_name_tool invalidated its signature.
# Expanded below as ${STAGED_EXTRA[@]+...} rather than plain "${STAGED_EXTRA[@]}": macOS ships
# bash 3.2 as /bin/bash, where expanding an empty array trips set -u. EXIFTOOL_MACHO is spelled
# the same way for the same reason.
STAGED_EXTRA=()
if [[ -n "${MODULES_DIR}" ]]; then
    while IFS= read -r -d '' f; do
        STAGED_EXTRA+=("${f}")
    done < <(find "${MODULES_DIR}" -type f -print0)
fi
if [[ -n "${VIPS_PROBE}" ]]; then
    STAGED_EXTRA+=("${VIPS_PROBE}")
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
for f in "${BINARY}" "${DEST_DIR}/ffmpeg" "${DEST_DIR}/ffprobe" "${LIBS_DIR}"/* ${STAGED_EXTRA[@]+"${STAGED_EXTRA[@]}"}; do
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
for f in "${BINARY}" "${DEST_DIR}/ffmpeg" "${DEST_DIR}/ffprobe" "${LIBS_DIR}"/* ${STAGED_EXTRA[@]+"${STAGED_EXTRA[@]}"} ${EXIFTOOL_MACHO[@]+"${EXIFTOOL_MACHO[@]}"}; do
    [[ -f "${f}" ]] || continue
    if [[ -n "${SIGN_IDENTITY}" ]]; then
        codesign --force --options runtime --timestamp --sign "${SIGN_IDENTITY}" "${f}"
    else
        codesign --force --sign - "${f}"
    fi
done

# Fail the build rather than ship a bundle that stores an .avif and never previews it -- the
# macOS counterpart of the libheif probe scripts/cross-build-rpi/avif-decode-probe.c does for
# the Linux packages. Runs last, so what it measures is the signed bundle that actually ships.
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
FIXTURE="${REPO_ROOT}/common/ext/testdata/sample.avif"

echo "==> Checking the bundled libvips can decode AVIF"
if [[ -z "${VIPS_PROBE}" ]]; then
    echo "!! No vips executable under 'brew --prefix vips' -- cannot check what the bundled libvips decodes" >&2
    exit 1
fi
if [[ ! -f "${FIXTURE}" ]]; then
    echo "!! Missing fixture ${FIXTURE}" >&2
    exit 1
fi
if ! VIPSHOME="${DEST_DIR}" "${VIPS_PROBE}" --version >/dev/null 2>&1; then
    echo "!! ${VIPS_PROBE} will not run on this host, so the bundle cannot be checked." >&2
    echo "   If this is the amd64 bundle being cross-built on Apple Silicon, running it needs" >&2
    echo "   Rosetta 2: softwareupdate --install-rosetta" >&2
    exit 1
fi
if ! VIPSHOME="${DEST_DIR}" "${VIPS_PROBE}" thumbnail "${FIXTURE}" "${PROBE_TMP}/avif.jpg" 320 \
        >"${PROBE_TMP}/err" 2>&1; then
    echo "!! The bundled libvips cannot decode AVIF:" >&2
    sed 's/^/   /' "${PROBE_TMP}/err" >&2
    echo "   Modules staged: ${MODULES_DIR:-none}" >&2
    echo "   Either the heif module did not make it into the bundle, or the libheif it needs" >&2
    echo "   has no AV1 decoder. Check what Homebrew's own vips reports first:" >&2
    echo "     vips header -f heif-compression ${FIXTURE}" >&2
    exit 1
fi
echo "    loader:  $(VIPSHOME="${DEST_DIR}" "${VIPS_PROBE}" header -f vips-loader "${FIXTURE}")"
# grep -m1 closes the pipe early, so vips exits on SIGPIPE and pipefail calls the pipeline
# failed even when the match was found -- capture first, then decide what to print.
modules_found="$(VIPSHOME="${DEST_DIR}" "${VIPS_PROBE}" --vips-info -l 2>&1 | grep -m1 'vips-modules-' || true)"
echo "    modules: ${modules_found:-none reported}"
# No HEVC fixture in the repo to decode, so this reports rather than asserts: without libde265
# an iPhone HEIC still previews, but through ffmpeg (common/preview.WithDecodeFallback).
de265=("${LIBS_DIR}"/*de265*)
if [[ -e "${de265[0]}" ]]; then
    echo "    HEVC:    libde265 bundled, so HEIC decodes in libvips too"
else
    echo "    HEVC:    no libde265 in libs/ -- HEIC previews will keep going through ffmpeg"
fi

echo "==> Done staging macOS dependencies into ${DEST_DIR}"
