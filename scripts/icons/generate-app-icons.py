#!/usr/bin/env python3
"""Regenerates the committed desktop and web app icons from one square source artwork.

Usage: python scripts/icons/generate-app-icons.py <source.png>

Writes:
  cmd/lomod/static/lomo/img/logo-64.png   web navigation mark
  cmd/lomod/static/lomo/img/logo-128.png  login and welcome page mark
  cmd/lomod/static/lomo/img/favicon.ico  browser icon (16..64)
  installers/macos/AppIcon-1024.png        artwork inset on Apple's 1024 grid (824px body),
                                           turned into AppIcon.icns by build-app-bundle.sh
  installers/windows/lomorage.ico          16..256 multi-size .ico for the Start menu /
                                           Desktop shortcuts and the tray icon

The source is expected to be the icon body on a transparent background; stray low-alpha
specks around it (common in generated artwork) are dropped before cropping. Needs Pillow
and numpy.
"""
import os
import sys

import numpy as np
from PIL import Image

REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))


def clean_master(src_path):
    rgba = np.array(Image.open(src_path).convert("RGBA")).astype(np.float32)
    alpha = rgba[:, :, 3]
    alpha[alpha < 32] = 0
    # The body's interior alpha sits a hair under 255 (~253); make it fully opaque. Scale
    # against a fixed threshold, not alpha.max(): a single stray 255 pixel would make that a
    # no-op.
    alpha[:] = np.clip(alpha * 255.0 / 250, 0, 255)
    ys, xs = np.where(alpha > 0)
    body = Image.fromarray(rgba.astype(np.uint8), "RGBA").crop(
        (xs.min(), ys.min(), xs.max() + 1, ys.max() + 1))
    side = max(body.size)
    square = Image.new("RGBA", (side, side), (0, 0, 0, 0))
    square.paste(body, ((side - body.width) // 2, (side - body.height) // 2))
    return square.resize((1024, 1024), Image.LANCZOS)


def write_web_icons(master):
    web_dir = os.path.join(REPO_ROOT, "cmd", "lomod", "static", "lomo", "img")
    os.makedirs(web_dir, exist_ok=True)
    for size in (64, 128):
        master.resize((size, size), Image.LANCZOS).save(
            os.path.join(web_dir, f"logo-{size}.png"), optimize=True)
    master.resize((64, 64), Image.LANCZOS).save(
        os.path.join(web_dir, "favicon.ico"),
        sizes=[(16, 16), (24, 24), (32, 32), (48, 48), (64, 64)],
        bitmap_format="bmp")


def main():
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    master = clean_master(sys.argv[1])
    write_web_icons(master)

    mac = Image.new("RGBA", (1024, 1024), (0, 0, 0, 0))
    mac.paste(master.resize((824, 824), Image.LANCZOS), (100, 100))
    mac.save(os.path.join(REPO_ROOT, "installers", "macos", "AppIcon-1024.png"), optimize=True)

    # Windows icons conventionally fill (nearly) the whole cell. bitmap_format="bmp": Pillow
    # defaults to PNG-compressed frames, which the tray's Windows PowerShell 5.1 / .NET
    # Framework System.Drawing.Icon decodes into garbage pixels.
    master.resize((256, 256), Image.LANCZOS).save(
        os.path.join(REPO_ROOT, "installers", "windows", "lomorage.ico"),
        sizes=[(16, 16), (20, 20), (24, 24), (32, 32), (40, 40), (48, 48), (64, 64), (128, 128), (256, 256)],
        bitmap_format="bmp")


if __name__ == "__main__":
    main()
