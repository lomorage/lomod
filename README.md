# lomod

The Lomorage server: a self-hosted photo and video backup service for your own
hardware (Raspberry Pi, Linux, Windows, macOS). The Lomorage apps back up to it
over your home network.

## Install

- Debian / Ubuntu / Raspberry Pi OS, Fedora, Rocky, openSUSE: see
  the [installation guides](https://lomorage.com/docs/Installation/lomorage-service/)
  for the apt/rpm repositories.
- Windows: `irm https://lomorage.com/windows/install.ps1 | iex`
- macOS: `curl -fsSL https://lomorage.com/mac/install.sh | bash`

## Layout

| Path | What |
|---|---|
| `cmd/lomod` | the server |
| `cmd/lomoupg` | the self-updater used by the Windows and macOS installs |
| `cmd/lomoc` | command-line client (import, check, migrate) |
| `handler/`, `common/` | HTTP handlers and shared packages |
| `migrations/` | database schema migrations |
| `rpms-build/` | Debian/RPM/Docker packaging files |
| `installers/` | the Windows and macOS install scripts served from lomorage.com |
| `scripts/` | cross-build and release tooling |

## Build and test

Go modules are vendored (`-mod=vendor`), and building needs libvips and cgo.

```
make build-lomod      # Linux
make test-handler     # handler integration tests
```

Linux packages for arm/amd64 are cross-built in Docker, see
[scripts/cross-build-rpi/README.md](scripts/cross-build-rpi/README.md). Windows and
macOS builds and the full release flow are in [docs/releasing.md](docs/releasing.md).

## License

[MIT](LICENSE)
