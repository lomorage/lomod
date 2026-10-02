# Releasing lomod

Every platform is released with the same script: `scripts/release-all.sh`.

| Platform | Run on | Published to |
|----------|--------|--------------|
| linux | Windows (git-bash, built through WSL2 + Docker) | the apt / rpm repositories in lomoware.github.io |
| windows | Windows (git-bash, built natively) | a GitHub Release (lomosw/lomosw.github.io) + `windows-cli` in `release.json` |
| macos | Mac (Apple Silicon, builds both arm64 and amd64) | a GitHub Release (lomosw/lomosw.github.io) + `macos-cli-arm64` / `macos-cli-amd64` in `release.json` |

`release.json` lives in the homepage repo as `static/release.json` and is deployed to `https://lomorage.com/release.json`. Installed machines update from it every day (Windows: a scheduled task plus a check by the tray at logon; macOS: a LaunchAgent). Linux machines upgrade from the package repositories every day with `apt` / `dnf`.

**A release goes to every user.** Only release when there is a change worth shipping, and install a local build on your own machine first (see "Testing before a release" below).

## Usage

```bash
bash scripts/release-all.sh                    # on Windows: linux + windows; on the Mac: macos
bash scripts/release-all.sh --only windows     # only some platforms, comma separated: --only linux,windows
bash scripts/release-all.sh --dry-run          # print the plan only; no checks, nothing published
make release-all RELEASE_ARGS="--only windows" # same, through make
```

If only one platform changed, release only that platform: a change to the Windows tray doesn't need new Linux packages.

Choosing a platform this machine can't build is an error (Windows can't release macos; the Mac can't release linux / windows).

## What the script does

1. **Preflight**
   - `gh` is logged in; the tools each platform needs are present; for macOS, the signing identity and notarytool profile are set up.
   - HEAD is pushed, CI passed on it, and there are no uncommitted changes (`SKIP_CI_CHECK=1` skips this in an emergency).
   - Syncs `installers/windows/install.ps1` and `installers/macos/install.sh` into homepage and runs homepage's `npm test`. Any failure stops before anything is published.
2. **Build and publish each platform** (as chosen by `--only`)
   - linux: `build-all.sh` in WSL, then sign and push the apt / rpm repositories.
   - windows: `make release-windows`, then create the GitHub Release.
   - macos: `make release-macos` and `make release-macos-amd64` (building and signing are done by `scripts/macos/build-app-bundle.sh` and `scripts/macos/collect-deps.sh`), submit each architecture to Apple notarization and wait for the result, then create one GitHub Release with both tarballs.
3. **Update homepage**: `scripts/update-release-manifest.py` updates this release's keys in `release.json`, which gets committed and pushed together with the synced installers. If the push is rejected (usually because a release from the other machine just landed), the script rebases and pushes again; it only stops, leaving it to you, when both changed the same key.

## One-time setup

### Both machines

- lomod and homepage checked out side by side in the same directory (the script looks for `../homepage`; `HOMEPAGE_DIR` overrides it).
- `gh auth login`, with an account that can create Releases in lomosw/lomosw.github.io.
- node installed for homepage, and `npm ci` run there once.
- git can push homepage over SSH (`git@github.com:lomorage/homepage.git`).

### Windows

- WSL2 Ubuntu + Docker and the cross-compile toolchain (see `scripts/cross-build-rpi/README.md`), and the `lomorage@gmail.com` GPG signing key imported into WSL's `~/.gnupg`.
- go, mingw64, pkgconfig-lite-raw and vips staged under `windows-deps/` (see `scripts/windows/fetch-vips.ps1`).
- exiftool in `windows-deps/exiftool-<version>/`. Builds use it directly and no longer go to SourceForge. The first build downloads it, checks it against a pinned SHA256 and puts it there. If SourceForge is down, take `exiftool.exe` and `exiftool_files/` out of the previous Windows release zip and put them there (check the zip against the SHA256 in `release.json` first).

### Mac

- Go; the Xcode Command Line Tools (`swiftc`, `lipo`, `iconutil`, `codesign`, `xcrun notarytool`).
- arm64 Homebrew (`/opt/homebrew`): `brew install vips ffmpeg exiftool dylibbundler`.
- x86_64 Homebrew (`/usr/local`, installed under Rosetta): `vips ffmpeg exiftool`. The amd64 tarball takes its dependencies from here.
- Signing identity: a Developer ID Application certificate in the login keychain, with its name written to `.macos-sign-identity` at the repo root:
  ```bash
  security find-identity -v -p codesigning          # find "Developer ID Application: ..."
  echo -n "Developer ID Application: WTAO LLC (3GRDMJ5JMM)" > .macos-sign-identity
  ```
- A notarytool profile for notarization, with its name written to `.macos-notary-profile` at the repo root:
  ```bash
  xcrun notarytool store-credentials <profile-name>    # skip if you already have one
  echo -n "<profile-name>" > .macos-notary-profile
  xcrun notarytool history --keychain-profile "$(cat .macos-notary-profile)"   # works if it lists submissions
  ```
  Both files are in `.gitignore` and never committed.

### First release from the Mac with this script

Notarization, creating the Release and updating `release.json` for macOS were only added to the script on 2026-10-01; before that they were done by hand. So before the first release:

1. `git pull` to get the latest `main`.
2. Write `.macos-sign-identity` and `.macos-notary-profile`.
3. Run `bash scripts/release-all.sh --dry-run` and check the plan is macos only.
4. Run it for real. Keep the output if something fails. If notarization is rejected, the script tells you to run `xcrun notarytool log <id> --keychain-profile <profile>` to see why.

## Testing before a release

Build locally and install it yourself, without releasing:

- **Windows**: `make release-windows` (the zip lands in `releases/windows/`). Stop lomod from the tray first (a running `lomod.exe` can't be overwritten), then unzip it over `%LOCALAPPDATA%\Lomorage\lomod`. The zip has no `lomod.args` or `version.txt`, so those two files are kept. Then Start it and use it for a day or two.
- **macOS**: `make release-macos` (the tarball lands in `releases/macos/`). Stop lomod from the menu bar, copy the contents of `dist-macos/` over the install directory (its path is in `~/Library/Application Support/Lomorage/tray-install-dir.txt`), then Start it.
- **Linux (test Pi)**: build the `.deb` with `build-all.sh` in WSL, `scp` it to the test Pi, `sudo dpkg -i` it, and watch `sudo journalctl -u lomod --since today`.

## Checking after a release

```bash
curl -s https://lomorage.com/release.json | python -m json.tool        # new version, URL, SHA256
gh release list --repo lomosw/lomosw.github.io --limit 5
```

On Windows / macOS you can also click "Check for Updates" in the tray menu on an installed machine; it should install the new version and show its version number. `release.json` is only updated once GitHub Actions finishes deploying homepage, about a minute after the push.

## Troubleshooting

**The release failed halfway**
Don't re-release platforms that already went out: apt / rpm versions and GitHub Release tags would collide. Use `--only` for the platforms that didn't finish. For example, if Linux went out and the Windows build failed, fix it and run `--only windows`.

**The homepage push was rejected**
The script rebases and pushes again by itself. If it reports a rebase conflict, two releases changed the same key: in homepage, look at what the other one changed with `git log --oneline HEAD..origin/master`, resolve it by hand and push. homepage's working tree often has dozens of files that show as modified only because of line endings; `git diff --ignore-cr-at-eol` confirms they aren't real changes.

**SourceForge downloads fail during the Windows build**
SourceForge went down on 2026-09-30, returning 522 or an HTML page. exiftool now comes from `windows-deps/exiftool-<version>/`, so normally SourceForge isn't contacted at all; if that directory is missing, put it there as described in "Windows" above.

**Re-running after an interruption**
If a build was killed partway, delete the truncated `handler/rice-box.go` first (it's gitignored and gets regenerated), then run again.

## Testing the release script itself

```bash
bash scripts/test-release-all.sh               # Windows git-bash: --only, release.json updates, homepage push and conflict handling
pwsh -File scripts/windows/test-collect-deps.ps1   # Windows dependency download cache and retries
pwsh -File scripts/windows/test-self-update.ps1    # Windows self-update and the tray menu (needs go)
```

None of these publish anything.

## Later: nightly / stable channels

Once there are more users, each release could go to nightly first, run on test machines for a while, and then be promoted to stable. The design is in `docs/release-channels.md` and is on hold for now; its first step (lomoupg `--only-newer`, so updates never downgrade) has already shipped.
