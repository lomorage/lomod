# Release channel design: nightly and stable

Status: on hold (2026-10-01). There aren't many users yet, so for now we use something simpler: release only when there's a change worth shipping, test it yourself first, and release only the platforms that changed (see `docs/releasing.md`). The first step (lomoupg `--only-newer`) has shipped; the rest waits until it's needed.

## Problem

Every run of `scripts/release-all.sh` is a full release:

- Windows: `windows-cli` in `lomorage.com/release.json` points at the new version, and every machine with the CLI installed updates itself from the daily scheduled task or the tray's logon check.
- macOS: the same with `macos-cli-<arch>`.
- Linux: the new packages go into the apt/rpm repositories in `lomoware.github.io`, and `lomod-update.timer` runs `apt-get --only-upgrade` / `dnf upgrade` every day.

So every build goes straight to all users, with no stage where it runs on a few machines for a few days first.

## Goals

1. By default every build goes only to the **nightly** channel, which only machines that opt in receive (dev machines, the test Pi, test users).
2. After a nightly build has run on test machines for a while, one command promotes **that same build** to **stable**, without recompiling. Regular users follow stable only.
3. Existing installs need no changes and keep following stable.

## Design

### 1. Windows / macOS: nightly keys in release.json

Each CLI key in `release.json` gets a `-nightly` twin:

```json
"windows-cli":               { "Version": "...", "URL": "...", "SHA256": "..." },
"windows-cli-nightly":       { "Version": "...", "URL": "...", "SHA256": "..." },
"macos-cli-arm64":           { ... },
"macos-cli-arm64-nightly":   { ... },
"macos-cli-amd64":           { ... },
"macos-cli-amd64-nightly":   { ... }
```

- **Release**: `release-all.sh` only writes `windows-cli-nightly`. The GitHub Release (lomosw/lomosw.github.io) is still created, but marked `--prerelease` instead of `--latest`.
- **Promote**: a new script, `scripts/promote.sh windows` (likewise `macos`), copies the three fields of `windows-cli-nightly` unchanged into `windows-cli`, then commits and pushes homepage; it also marks that GitHub Release as latest and clears prerelease. The URL and SHA256 don't change, so users get exactly the zip the test machines ran.
- **Choosing a channel at install time**:
  - Windows: `install.ps1` gets `-Channel stable|nightly` (also read from `$env:LOMOD_CHANNEL`, for `irm | iex`), defaulting to stable. It decides which ManifestKey to use.
  - macOS: `install.sh` gets `--channel` (also read from `LOMOD_CHANNEL`); the existing `--manifest-key` still works and takes precedence.
- **Remembering the channel for updates**:
  - Windows: the installer writes the ManifestKey to `channel.txt` in the install directory. `lomorage-update.ps1` reads `-ManifestKey` from that file by default, falling back to `windows-cli` (so old installs are unaffected). `lomorage-post-update.bat` restores `channel.txt` from the backup, as it does `lomod.args`. That way the scheduled task, the tray's logon check and the "Check for Updates" menu need no arguments.
  - macOS: the LaunchAgent plist already stores `--manifest-key`; install.sh just has to write the right one for the channel.
- **Tray**: the version line shows the channel, e.g. `Version 2026-... (nightly)`, so it's always clear what a test machine is running.

### 2. Linux: a `-nightly` repository per distribution codename

apt: add a distribution for each codename in `lomoware.github.io/debian/<codename>/conf/distributions`:

```
Codename: bookworm-nightly
Suite: nightly
Components: main
Architectures: armhf arm64 amd64
SignWith: 0CE12BA8
```

- **Release**: `publish-debian.sh` changes to `includedeb <codename>-nightly`.
- **Promote**: `reprepro copy <codename> <codename>-nightly lomo-backend` (likewise lomo-vips). reprepro's copy is made for exactly this: the package files are shared in the pool and aren't re-signed.
- **Choosing a channel**: nightly machines add one line to sources.list, `deb https://lomoware.lomorage.com/debian/bookworm bookworm-nightly main`. Nightly versions are newer, so apt picks them naturally; stable users don't have that line and never see nightly. `update-lomod` doesn't change.

rpm (fedora / rocky9 / opensuse):

- Add a `-nightly` directory next to each repository directory (e.g. `rpm/fedora-nightly`), and have `publish-*.sh` write there.
- **Promote**: copy the rpms into the stable directory, re-run `createrepo_c` and sign repomd.
- Nightly machines install one extra `.repo` file pointing at the `-nightly` directory.

### 3. release-all.sh

- Releases to nightly by default. The process is the same as now; only the targets change to the nightly keys, repositories and directories above.
- New `scripts/promote.sh <windows|macos|linux|all>`: reads which version nightly currently points at, prints it for confirmation, then does the promotion for each platform as above. It only edits metadata and copies already-signed packages, takes seconds, and needs neither Docker nor the toolchains.

## Downgrades

lomoupg used to "upgrade" whenever the version was merely **different** (`cmd/lomoupg/main.go` only skipped when `p.Version == curr-version`). So a nightly machine switching back to stable would immediately install the older stable version, and an older lomod can't always read a database a newer one has changed.

Fix: lomoupg's `-only-newer` (shipped). Versions look like `YYYY-MM-DD.HH-MM-SS.…`, so string order is time order, and the update scripts always pass it. A machine switching back to stable then stays on its current nightly until stable catches up, and follows stable from there. apt/dnf never downgrade automatically anyway, so Linux needs no change.

## Migration

- Existing Windows / macOS installs: no `channel.txt`, so they default to `windows-cli` / `macos-cli-<arch>`, i.e. stable; behavior unchanged.
- Existing Linux installs: sources.list only has the stable line; behavior unchanged.
- First rollout: write the current stable version into every nightly key and repository too, so nightly has a starting point; from then on release-all.sh writes nightly only.
- Switch our own test machines (the Windows dev machine, the test Pi) to nightly.

## Implementation order

1. lomoupg `-only-newer`, passed by all update scripts. Done. It had to ship first: it must already be on users' machines before machines can safely switch channels.
2. Windows: `channel.txt`, install.ps1 `-Channel`, restore in post-update, channel in the tray; `windows-cli-nightly` in release.json.
3. Linux: `-nightly` distributions and rpm directories, publish scripts write there.
4. `promote.sh`, and release-all.sh releasing nightly only.
5. macOS: install.sh `--channel`, and the macOS release writes the nightly keys.

Every step comes with real integration tests: on Windows, extend the sandbox in `scripts/windows/test-self-update.ps1` (a local release.json with both nightly and stable keys, covering switching and no-downgrade); on Linux, actually switch sources on the test Pi.

## Open questions

- How long must a nightly run on test machines before it's promoted? Suggestion: no fixed rule; promote.sh prints how long ago the nightly was released and a person decides.
- Should nightly be offered to test users publicly? If so, the install instructions on lomorage.com need a section on it.
- LomoAgent's own `windows` / `darwin` keys are out of scope.
