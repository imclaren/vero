#!/usr/bin/env python3
"""Lays the per-platform GIFs in docs/screenshots out as a grid, one tile per
README row, all playing at once - the README's table as a single image.

    python3 scripts/lib/stitch-gifs.py docs/screenshots/all-platforms.gif

Each tile is the platform's name over its clip, scaled to the table's 300px
width.  Solaris and AIX have no clip, and get the words the table gives them.
The clips have different lengths, so each loops inside the longest.  Pillow
lays the frames out and ffmpeg builds the palette, which it does better.
"""
import os
import subprocess
import sys
import tempfile

from PIL import Image, ImageDraw, ImageFont, ImageSequence

HERE = os.path.dirname(os.path.abspath(__file__))
SHOTS = os.path.join(HERE, "..", "..", "docs", "screenshots")

# The README's row order, and its notes.  None for the clip means a row that
# builds but has nothing to show.
TILES = [
    ("macos",     "macOS",     "SwiftUI, in the menu bar"),
    ("windows",   "Windows",   "WPF"),
    ("linux",     "Linux",     "GTK4"),
    ("freebsd",   "FreeBSD",   "GTK4"),
    ("openbsd",   "OpenBSD",   "GTK4"),
    ("netbsd",    "NetBSD",    "GTK4"),
    ("dragonfly", "DragonFly", "GTK4"),
    ("illumos",   "illumos",   "GTK4, on OpenIndiana"),
    ("android",   "Android",   "Kotlin, in the emulator"),
    ("ios",       "iOS",       "SwiftUI, in the Simulator"),
    ("web",       "Browser",   "js/wasm, the worker compiled in"),
    ("wasi",      "WASI",      "wasip1/wasm, under wasmtime"),
    ("plan9",     "Plan 9",    "libdraw, in a rio window"),
    (None,        "Solaris",   "solaris/amd64"),
    (None,        "AIX",       "aix/ppc64"),
]

COLUMNS = 3
TILE_W, TILE_H = 300, 158     # the README's 300px, at the clips' 480:252
STRIP = 26                    # the name above each clip
GUTTER = 12
FPS = 4                       # output frames per second; clips are 2 or 3
FONT = "/System/Library/Fonts/Helvetica.ttc"

BOLD = ImageFont.truetype(FONT, 13, index=1)
PLAIN = ImageFont.truetype(FONT, 12)


class Clip:
    """A GIF's frames and the time each one starts, so a frame can be looked
    up for any moment; the clip repeats once it runs out."""

    def __init__(self, path):
        self.frames, self.starts, t = [], [], 0
        with Image.open(path) as im:
            for frame in ImageSequence.Iterator(im):
                self.frames.append(fit(frame.convert("RGB")))
                self.starts.append(t)
                t += frame.info.get("duration", 100) / 1000
        self.length = t

    def at(self, t):
        t %= self.length
        i = 0
        while i + 1 < len(self.starts) and self.starts[i + 1] <= t:
            i += 1
        return self.frames[i]


def fit(frame):
    """The frame scaled to the tile and centred on white."""
    scale = min(TILE_W / frame.width, TILE_H / frame.height)
    size = (round(frame.width * scale), round(frame.height * scale))
    tile = Image.new("RGB", (TILE_W, TILE_H), "white")
    tile.paste(frame.resize(size, Image.LANCZOS),
               ((TILE_W - size[0]) // 2, (TILE_H - size[1]) // 2))
    return tile


def words(platform, note):
    """A tile for a row with no clip: what the table says of it."""
    note = "Builds, and does not run here"
    tile = Image.new("RGB", (TILE_W, TILE_H), (250, 250, 250))
    draw = ImageDraw.Draw(tile)
    draw.rectangle([0, 0, TILE_W - 1, TILE_H - 1], outline=(225, 225, 225))
    w = draw.textlength(note, font=PLAIN)
    draw.text(((TILE_W - w) / 2, TILE_H / 2 - 7), note, font=PLAIN, fill=(130, 130, 130))
    return tile


def strip(platform, note):
    s = Image.new("RGB", (TILE_W, STRIP), "white")
    draw = ImageDraw.Draw(s)
    draw.text((0, 5), platform, font=BOLD, fill=(20, 20, 20))
    x = draw.textlength(platform, font=BOLD) + 6
    draw.text((x, 6), "— " + note, font=PLAIN, fill=(120, 120, 120))
    return s


def main(out):
    clips = {name: Clip(os.path.join(SHOTS, name + ".gif"))
             for name, _, _ in TILES if name}
    rows = -(-len(TILES) // COLUMNS)
    width = COLUMNS * TILE_W + (COLUMNS + 1) * GUTTER
    height = rows * (STRIP + TILE_H) + (rows + 1) * GUTTER
    strips = [strip(p, n) for _, p, n in TILES]
    still = {i: words(p, n) for i, (c, p, n) in enumerate(TILES) if not c}

    length = max(c.length for c in clips.values())
    count = int(round(length * FPS))
    work = tempfile.mkdtemp()
    listing = []
    for f in range(count):
        t = f / FPS
        canvas = Image.new("RGB", (width, height), "white")
        for i, (name, _, _) in enumerate(TILES):
            x = GUTTER + (i % COLUMNS) * (TILE_W + GUTTER)
            y = GUTTER + (i // COLUMNS) * (STRIP + TILE_H + GUTTER)
            canvas.paste(strips[i], (x, y))
            canvas.paste(clips[name].at(t) if name else still[i], (x, y + STRIP))
        png = os.path.join(work, "f%04d.png" % f)
        canvas.save(png)
        listing.append("file '%s'\nduration %.3f" % (png, 1 / FPS))
    # The concat demuxer drops the last duration unless the file repeats.
    listing.append("file '%s'" % png)
    with open(os.path.join(work, "frames.txt"), "w") as f:
        f.write("\n".join(listing) + "\n")

    subprocess.run([
        "ffmpeg", "-loglevel", "error", "-y",
        "-f", "concat", "-safe", "0", "-i", os.path.join(work, "frames.txt"),
        "-vf", "split[a][b];[a]palettegen=stats_mode=diff[p];[b][p]paletteuse=dither=none",
        "-loop", "0", out,
    ], check=True)
    print("wrote %s: %dx%d, %d frames, %.1fs" % (out, width, height, count, length))


if __name__ == "__main__":
    main(sys.argv[1] if len(sys.argv) > 1 else os.path.join(SHOTS, "all-platforms.gif"))
