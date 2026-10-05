"""Writes a Windows .ico from one large PNG: the sizes Windows uses, each a
PNG inside the icon (Windows Vista and after read those), resized by
macOS's sips. No ImageMagick, nothing to install.

    python3 lib/ico.py icon.png out.ico
"""

import os
import struct
import subprocess
import sys
import tempfile

SIZES = [16, 24, 32, 48, 64, 128, 256]


def main(src: str, out: str) -> None:
    images = []
    with tempfile.TemporaryDirectory() as tmp:
        for size in SIZES:
            path = os.path.join(tmp, f"{size}.png")
            subprocess.run(["sips", "-z", str(size), str(size), src, "--out", path],
                           check=True, capture_output=True)
            with open(path, "rb") as f:
                images.append((size, f.read()))
    header = struct.pack("<HHH", 0, 1, len(images))
    offset = len(header) + 16 * len(images)
    entries, data = b"", b""
    for size, png in images:
        # 256 is written as 0: the field is a byte.
        entries += struct.pack("<BBBBHHII", size % 256, size % 256, 0, 0, 1, 32, len(png), offset + len(data))
        data += png
    with open(out, "wb") as f:
        f.write(header + entries + data)


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
