# Cross-building lomod/lomoc for Raspberry Pi (+ amd64) from Windows

Builds `lomod` and `lomoc` for armhf, arm64, and amd64, all linked against
Debian **buster (10)**'s glibc so the resulting binaries run unmodified on
Debian 10 through 13 (glibc is forward-compatible). Runs from Windows via
WSL2. armhf/arm64 use real cross-compilation (native amd64
`gcc-aarch64-linux-gnu` / `gcc-arm-linux-gnueabihf`) instead of QEMU
emulation, so it's fast; amd64 builds natively inside a buster container
(see `amd64/` below).

## One-time setup (run manually inside a WSL2 Ubuntu terminal — needs sudo)

```
wsl -d Ubuntu
bash scripts/cross-build-rpi/00-setup-docker.sh      # docker + buildx + qemu binfmt (only needed to extract sysroots below)
bash scripts/cross-build-rpi/01-setup-cross-gcc.sh    # native cross-compilers + pkg-config
bash scripts/cross-build-rpi/02-extract-sysroots.sh   # pulls lomorage/vips-*-debian-buster-build images,
                                                       # extracts /usr and /lib into ~/sysroots/{aarch64,armhf},
                                                       # and rewrites absolute symlinks to be sysroot-relative
```

`02-extract-sysroots.sh` is the one that matters long-term: it gets you
Debian buster's vips/sqlite3/glibc headers and libraries (as built by
lomorage in 2021) without needing buster's now-archived apt repos. Re-run it
if `~/sysroots` ever gets wiped; the other two are pure package installs and
rarely need to run twice.

## Building (repeatable, no sudo, ~native speed)

```
wsl -d Ubuntu
bash scripts/cross-build-rpi/build.sh arm64     # -> /tmp/lomod-arm64, /tmp/lomoc-arm64
bash scripts/cross-build-rpi/build.sh armhf     # -> /tmp/lomod-armhf, /tmp/lomoc-armhf
bash scripts/cross-build-rpi/package.sh         # -> releases/lomod_<version>_{armhf,arm64}.deb
```

Or from PowerShell without opening a WSL shell yourself:

```
wsl -d Ubuntu -- bash /mnt/c/Users/fuji2/Work/lomo-backend/scripts/cross-build-rpi/build.sh arm64
wsl -d Ubuntu -- bash /mnt/c/Users/fuji2/Work/lomo-backend/scripts/cross-build-rpi/build.sh armhf
wsl -d Ubuntu -- bash /mnt/c/Users/fuji2/Work/lomo-backend/scripts/cross-build-rpi/package.sh
```

## Why the extra flags (`build.sh` internals)

Getting a correct link against an old (2021-era buster) sysroot from a
modern (2024+) Ubuntu cross-toolchain took a few non-obvious fixes; they're
baked into `build.sh` so you shouldn't need to touch them, but if the build
breaks again, here's what each one is for:

- **Absolute symlinks in the extracted sysroot** (e.g.
  `libresolv.so -> /lib/aarch64-linux-gnu/libresolv.so.2`) escape `--sysroot`
  entirely, because the kernel resolves the literal absolute target against
  the real root, not the extracted tree. `02-extract-sysroots.sh` rewrites
  every absolute symlink under the sysroot to an equivalent sysroot-relative
  one (`ln -sfr`) right after extraction.
- **`-static-libgcc`**: without it, the linker pulls in the *host*
  cross-toolchain's own `libgcc_s.so.1` (built for a much newer glibc), which
  requires versioned symbols (`pthread_setspecific@GLIBC_2.34`, etc.) that
  don't exist in buster's glibc 2.28. Static linking avoids that dependency.
- **`-tags netgo`**: buster's libresolv in this sysroot only exports
  `__res_search`, not the plain `res_search` alias Go's cgo net resolver
  expects. `netgo` switches to Go's pure-Go DNS resolver, sidestepping the
  cgo resolver path (and its `res_search` symbol) entirely. CGO stays on for
  sqlite3/vips.
- **`-D_TIME_BITS=32`** (armhf only): Ubuntu 24.04's 32-bit ARM cross-gcc
  defaults to the 64-bit `time_t` ABI (`__localtime64` etc.), which doesn't
  exist in buster's glibc. This forces the classic 32-bit time API buster
  actually has.
- **`rice embed-go` before every `go build`**: without it, the binary links
  fine and passes `--version`/`ldd` checks, but panics the moment it
  actually starts (`could not locate box "../cmd/lomod/static"`) --
  `rice.MustFindBox` needs either this generated file (baking the static UI
  assets into the binary) or the real `static/` directory sitting next to
  the binary, which won't exist in a packaged install. This is exactly why
  a real service-start test (not just `--version`) matters -- this bug
  shipped undetected through every smoke test in an earlier round of this
  work and only surfaced on real hardware.

## amd64

Unlike arm64/armhf, no pre-built Debian-buster amd64 image with vips exists
(the `lomorage/vips-*-debian-buster-build` images only cover the two ARM
targets), and buster's own apt only ships vips 8.7.4 — too old, and missing
features (HEIC etc.) the ARM builds have. So `amd64/Dockerfile.buster-build`
builds vips 8.10.6 from source (apt-installable build deps only — no other
from-source dependencies needed) plus a modern Go toolchain, inside a
`debian:buster` image. buster's mirrors are EOL, so it points apt at
`archive.debian.org` instead. Since the WSL host is already amd64, no
cross-compilation is needed here — it's a native build inside the container:

```
wsl -d Ubuntu
docker build -t lomod-amd64-buster-build -f scripts/cross-build-rpi/amd64/Dockerfile.buster-build scripts/cross-build-rpi/amd64
bash scripts/cross-build-rpi/amd64/build.sh   # -> scripts/cross-build-rpi/amd64/out/{lomod,lomoc}-amd64
```

The Docker image only needs building once (or whenever you want to bump the
vips/Go version); `build.sh` re-runs fast against the cached image.

## Packaging

`package.sh` reuses the packaging metadata in `rpms-build/lomod`
— control files, systemd units (lomod itself plus the daily `lomod-update`
timer that runs `/opt/lomorage/bin/update-lomod`), postinst/preinst — and just drops
the freshly-built binaries in.

It packages armhf/arm64 unconditionally, and also amd64 if
`amd64/out/lomod-amd64` exists (i.e. you ran the amd64 build above first).

No `sudo` or Docker needed for packaging itself; `dpkg-deb --build
--root-owner-group` builds a valid `.deb` as a normal user.

**Important:** `rpms-build/lomod`'s `preinst`/`postinst`/update script and
unit files used to check out with CRLF line endings on Windows (git `autocrlf`;
`.gitattributes` now forces LF for them), which silently breaks their shebang (`dpkg` fails with "unable
to execute ... No such file or directory" trying to exec `/bin/bash\r`).
`package.sh` strips `\r` from those specific files after copying them in —
if you ever see that dpkg error again, that's almost certainly why; check
`file DEBIAN/preinst` for "CRLF line terminators".

## Extending to more distro codenames (jammy/noble/trixie, etc.)

Confirmed empirically (see `lomoware.github.io` commit history), not just
assumed: a `lomo-backend`/`lomo-vips` pair built against one Debian release
generally **also works on later Debian releases** without rebuilding,
because Debian has kept the relevant library SONAMEs (webp/tiff/
ImageMagick/glibc) stable release over release — buster→bookworm→trixie all
work with the same `.deb`s. **This does not hold for Ubuntu**: Ubuntu bumps
those SONAMEs on its own schedule, so a Debian-built `lomo-vips` fails to
even start on Ubuntu (confirmed: `lomod` fails with "cannot open shared
object file" for `libwebp.so.6` on jammy, plus `libMagickCore`/`libtiff` on
noble). Each Ubuntu codename needs its own `lomo-vips` built natively
against that release's own library stack:

```
wsl -d Ubuntu
bash scripts/cross-build-rpi/amd64/build-vips-ubuntu.sh jammy ubuntu:22.04
bash scripts/cross-build-rpi/amd64/build-vips-ubuntu.sh noble ubuntu:24.04
```

Before publishing any new codename to the live apt repo, smoke-test it in a
throwaway container first — install both `.deb`s, then actually run
`lomod --version` (not just `dpkg -i`, which can succeed while the binary
still fails to load at runtime) — and don't try to shortcut it for a new
distro family without testing, since the Debian-reuse assumption above is
exactly the kind of thing that looks obviously fine and quietly isn't.

## RPM (Fedora done; Rocky/openSUSE not yet)

See `rpm/README.md`. Same binaries, same "reuse the shared library floor,
but verify with a real container test before trusting it" discipline —
Fedora ships `vips` directly via `dnf` and our binaries just work against
it; Rocky/openSUSE don't have `vips` in their base repos at all and need
the from-source treatment.
