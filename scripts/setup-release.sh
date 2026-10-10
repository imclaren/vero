#!/bin/sh
# Installs what vero release and the recordings need beyond the examples:
# makensis, which builds the Windows installer, and ffmpeg, which makes the
# GIFs and pictures of each system's recording.
set -e
for tool in makensis ffmpeg; do
    if command -v "$tool" >/dev/null 2>&1; then
        echo "$tool: there already"
    else
        echo "installing $tool"
        brew install "$tool"
    fi
done
