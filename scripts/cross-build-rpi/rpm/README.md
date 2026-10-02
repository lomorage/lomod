# RPM packaging (Fedora, Rocky 9, openSUSE Leap)

Same underlying binaries as the `.deb` pipeline (`../amd64/build.sh` must
have been run first to produce `../amd64/out/{lomod,lomoc}-amd64`) -- RPM
packaging just wraps them differently.

## Fedora

Fedora ships `vips` directly via `dnf` (8.18.3 as of writing), and the
existing buster-linked `lomod`/`lomoc` binaries run against it completely
unmodified -- confirmed with a container smoke test (install + actually run
`lomod --version`, not just check the install succeeded) before publishing.
No from-source vips build needed here, unlike Rocky/openSUSE/Ubuntu.

```
wsl -d Ubuntu
bash scripts/cross-build-rpi/amd64/build.sh          # if not already built
bash scripts/cross-build-rpi/rpm/package-fedora.sh    # -> releases/lomo-backend-*.fc*.x86_64.rpm
bash scripts/cross-build-rpi/rpm/publish-fedora.sh    # signs + createrepo_c into lomoware.github.io/rpm/fedora
```

`publish-fedora.sh` mounts your WSL `~/.gnupg` into the signing container
read-only and copies it to a writable path inside before signing (RPM/gpg
both refuse to operate on a read-only homedir). Needs the same signing key
as the apt repo (`lomorage@gmail.com`, key `0CE12BA8`) already imported.

**`ffmpeg` isn't in Fedora's base repos** (patent/licensing) -- it's in
`Requires:` anyway since lomod genuinely needs it, so users must enable
[RPM Fusion](https://rpmfusion.org/) first:
```
dnf install https://download1.rpmfusion.org/free/fedora/rpmfusion-free-release-$(rpm -E %fedora).noarch.rpm
```
This mirrors the project's existing pattern of asking users to add a
third-party repo before installing (the lomoware repo itself).

## Rocky Linux 9, openSUSE Leap

Neither ships `vips` in its base repos (checked, even with EPEL enabled for
Rocky), so both need a `lomo-vips` package built from source natively
against each release's own library stack -- same treatment as Ubuntu
jammy/noble (see `../amd64/build-vips-ubuntu.sh` for that pattern).
`Dockerfile.rocky9-vips-build` / `Dockerfile.opensuse-vips-build` do the
from-source vips build; `package-vips-generic.sh` wraps the result into an
RPM using `lomo-vips.spec.in`.

```
wsl -d Ubuntu
docker build -t lomo-vips-rocky9-build   -f scripts/cross-build-rpi/rpm/Dockerfile.rocky9-vips-build   scripts/cross-build-rpi/rpm
docker build -t lomo-vips-opensuse-build -f scripts/cross-build-rpi/rpm/Dockerfile.opensuse-vips-build scripts/cross-build-rpi/rpm

bash scripts/cross-build-rpi/amd64/build.sh              # if not already built
bash scripts/cross-build-rpi/rpm/package-rocky9.sh        # -> releases/lomo-backend-*.el9.x86_64.rpm
bash scripts/cross-build-rpi/rpm/package-opensuse.sh      # -> releases/lomo-backend-*.x86_64.rpm (no dist suffix)
bash scripts/cross-build-rpi/rpm/publish-rocky9.sh        # signs + createrepo_c into lomoware.github.io/rpm/rocky9
bash scripts/cross-build-rpi/rpm/publish-opensuse.sh      # signs + createrepo_c into lomoware.github.io/rpm/opensuse
```

Both `publish-*.sh` scripts copy **both** the `lomo-backend` and `lomo-vips`
RPMs into the same repo directory before running `createrepo_c`, so
dnf/zypper can resolve `lomo-backend`'s `Requires: lomo-vips` from the repo
itself.

Two pitfalls hit and fixed while bringing these up (both non-obvious, worth
knowing if this ever needs touching again):

- **`AutoReqProv: no` in `lomo-vips.spec.in` breaks the dependency chain.**
  It was originally set to dodge an RHEL/Rocky rpmbuild shebang-mangling
  error from vips's bundled `#!/usr/bin/python` scripts (fixed separately,
  see below), but it also suppresses the package's auto-`Provides:
  libvips.so.42()(64bit)` -- which `lomo-backend`'s RPM auto-detects as a
  `Requires` from scanning `lomod`/`lomoc`'s ELF `NEEDED` entries. With
  `AutoReqProv: no` set, that requirement could never resolve. It also
  suppresses `lomo-vips`'s own auto-`Requires` on the shared libs it
  dynamically links against (`libgsf`, `libheif`, `libMagickCore`, etc.),
  so installing `lomo-vips` alone wouldn't pull those in from the base repo
  either. Fix: leave `AutoReq`/`AutoProv` on their defaults (both enabled)
  in `lomo-vips.spec.in`.
- **RHEL/Rocky's default linker search path doesn't include
  `/usr/local/lib`** (openSUSE's does). `lomo-vips` installs there, so on a
  fresh Rocky 9 container `ldd` reported `libvips.so.42 => not found` even
  though the RPM installed cleanly and `ldconfig` ran in `%post`.
  `package-vips-generic.sh` now writes `/etc/ld.so.conf.d/lomo-vips.conf`
  (containing `/usr/local/lib`) into the buildroot, and it's in
  `lomo-vips.spec.in`'s `%files`; harmless on openSUSE where it was already
  redundant.
- The ambiguous-shebang rpmbuild rejection is handled separately in
  `package-vips-generic.sh` (rewrites `#!/usr/bin/python` to
  `#!/usr/bin/python3` in vips's bundled CLI scripts before packaging).

**`perl-Image-ExifTool` needs EPEL, `ffmpeg` needs RPM Fusion** on Rocky 9
(verified installable, same pattern as Fedora):
```
dnf install epel-release
dnf install https://download1.rpmfusion.org/free/el/rpmfusion-free-release-9.noarch.rpm
```

**openSUSE Leap's `zypper` doesn't trust our unsigned-at-build-time local
RPMs by default** during a smoke test install from a local file (`zypper
install /path/to/foo.rpm` complains "Package header is not signed!"); not
an issue for real users since they install from the signed, `gpgkey=`-
verified repo via `zypper ar`/`.repo` file, only relevant when testing an
unpublished RPM directly.
