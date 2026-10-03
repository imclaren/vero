#!/usr/bin/env python3
"""Joins the per-platform GIFs in docs/screenshots into one, in README order,
with a strip above each naming the platform.

    python3 scripts/lib/stitch-gifs.py docs/screenshots/all-platforms.gif

Every clip keeps its own frame timing; the sizes differ, so each is scaled to
fit one canvas and centred on white.  Pillow lays the frames out and ffmpeg
builds the palette, which it does better than Pillow does.
"""
import os
import subprocess
import sys
import tempfile

from PIL import Image, ImageDraw, ImageFont, ImageSequence

HERE = os.path.dirname(os.path.abspath(__file__))
SHOTS = os.path.join(HERE, "..", "..", "docs", "screenshots")

# The README's row order, and its notes.
CLIPS = [
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
]

WIDTH, HEIGHT = 480, 252      # the largest clip; the rest are scaled up to fit
STRIP = 30                    # the label above each clip
FONT = "/System/Library/Fonts/Helvetica.ttc"


def frames_of(path):
    with Image.open(path) as im:
        for frame in ImageSequence.Iterator(im):
            yield frame.convert("RGB"), frame.info.get("duration", 100)


def place(frame):
    """The frame scaled to fit the canvas, on white, under its label strip."""
    scale = min(WIDTH / frame.width, HEIGHT / frame.height)
    size = (round(frame.width * scale), round(frame.height * scale))
    frame = frame.resize(size, Image.LANCZOS)
    canvas = Image.new("RGB", (WIDTH, STRIP + HEIGHT), "white")
    canvas.paste(frame, ((WIDTH - size[0]) // 2, STRIP + (HEIGHT - size[1]) // 2))
    return canvas


def label(canvas, platform, note):
    draw = ImageDraw.Draw(canvas)
    draw.rectangle([0, 0, WIDTH, STRIP], fill=(245, 245, 245))
    draw.line([0, STRIP, WIDTH, STRIP], fill=(220, 220, 220))
    bold = ImageFont.truetype(FONT, 14, index=1)
    plain = ImageFont.truetype(FONT, 13)
    x = 12
    draw.text((x, 8), platform, font=bold, fill=(20, 20, 20))
    x += draw.textlength(platform, font=bold) + 8
    draw.text((x, 9), "— " + note, font=plain, fill=(110, 110, 110))


def main(out):
    work = tempfile.mkdtemp()
    listing = []
    n = 0
    for name, platform, note in CLIPS:
        path = os.path.join(SHOTS, name + ".gif")
        for frame, ms in frames_of(path):
            canvas = place(frame)
            label(canvas, platform, note)
            png = os.path.join(work, "f%04d.png" % n)
            canvas.save(png)
            listing.append("file '%s'\nduration %.3f" % (png, ms / 1000))
            n += 1
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
    print("wrote %s: %d frames from %d clips" % (out, n, len(CLIPS)))


if __name__ == "__main__":
    main(sys.argv[1] if len(sys.argv) > 1 else os.path.join(SHOTS, "all-platforms.gif"))
