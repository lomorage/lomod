#!/usr/bin/env python3
"""Sets CLI entries in homepage's static/release.json (served as lomorage.com/release.json).

Usage: update-release-manifest.py <release.json> <key> <version> <url> <sha256> [<key> ...]

Each entry becomes {"Version", "URL", "SHA256"}; every other key, including LomoAgent's own
"windows"/"darwin", is left exactly as it was. Run by scripts/release-all.sh for each platform
it publishes.
"""
import json
import sys


def main(argv):
    if len(argv) < 6 or (len(argv) - 2) % 4:
        sys.exit(__doc__)
    path, entries = argv[1], argv[2:]
    with open(path) as f:
        data = json.load(f)
    for i in range(0, len(entries), 4):
        key, version, url, sha256 = entries[i:i + 4]
        if key in ("windows", "darwin"):
            sys.exit(f"{key} belongs to LomoAgent, not the CLI installers")
        data[key] = {"Version": version, "URL": url, "SHA256": sha256}
    with open(path, "w") as f:
        json.dump(data, f, indent=2)
        f.write("\n")


if __name__ == "__main__":
    main(sys.argv)
