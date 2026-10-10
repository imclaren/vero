#!/bin/sh
# make-gif.sh DIR: DIR/frames (f0001.png, .xwd or .xwd.gz, a frame every
# half second) made into DIR/app.gif and DIR/app.png. test-repo.sh and
# record-mac.sh make every recording this way, so that each system's
# looks alike.
#
# app.gif is cropped to the window where the display is black around it,
# starts at the first frame with the window drawn, plays at two frames a
# second with three of every four repeated frames left out, so that a wait
# goes by quickly but can still be seen, holds its last frame for three
# seconds, and is no wider than 640. It is not dithered: an app's flat
# colours need none, and dithering makes every frame differ, which a GIF
# pays for in size.
#
# app.png is the last frame, cropped the same way and no wider than 1280:
# the app after its steps, for a page or a README.
dir=${1:?the folder of a recording}
f="$dir/frames"
command -v ffmpeg >/dev/null || { echo "   (no ffmpeg here, so no GIF: brew install ffmpeg)"; exit 0; }
ls "$f" 2>/dev/null | grep -q . || exit 0
for g in "$f"/*.gz; do [ -e "$g" ] && gunzip -f "$g"; done
# Empty, from before the display was up: ffmpeg skips them, which would put
# its frame numbers out of step with the files.
find "$f" -type f -size -100c -delete
ls "$f" | grep -q . || { echo "   (no frames came back, so no GIF)"; exit 0; }
ext=$(ls "$f" | head -1 | sed 's/.*\.//')
last=$(ls "$f"/*."$ext" | tail -1)
# The display is black around the window: the last frame says where it is.
crop=$(ffmpeg -hide_banner -i "$last" -vf cropdetect=limit=0.01:round=2:skip=0 -f null - 2>&1 | grep -o 'crop=[0-9:]*' | tail -1)
# From the first frame with the app drawn in it: before that its part of
# the display is black, or partly, while the window maps. An app dark all
# over would lose every frame that way, so it keeps them.
black=$(ffmpeg -hide_banner -i "$f/f%04d.$ext" -vf "${crop:+$crop,}blackframe=amount=30:threshold=32" -f null - 2>&1 | grep -o 'frame:[0-9]*' | cut -d: -f2)
n=0 drop=""
for frame in $(ls "$f"); do
    echo "$black" | grep -qx "$n" || break
    drop="$drop $frame"
    n=$((n + 1))
done
[ "$n" -lt "$(ls "$f" | wc -l)" ] && for frame in $drop; do rm "$f/$frame"; done
i=0
for frame in $(ls "$f"); do
    i=$((i + 1))
    mv "$f/$frame" "$f/g$(printf %04d $i).$ext"
done
for frame in "$f"/g*; do mv "$frame" "$f/f${frame##*/g}"; done
last=$(ls "$f"/*."$ext" | tail -1)
ffmpeg -loglevel error -y -framerate 2 -i "$f/f%04d.$ext" \
    -vf "${crop:+$crop,}mpdecimate=max=3,setpts=N/2/TB,tpad=stop_mode=clone:stop_duration=3,scale='min(640,iw)':-2:flags=lanczos,split[a][b];[a]palettegen=max_colors=128[p];[b][p]paletteuse=dither=none:diff_mode=rectangle" \
    -fps_mode vfr -loop 0 "$dir/app.gif" || echo "   (the GIF could not be made)"
ffmpeg -loglevel error -y -i "$last" -vf "${crop:+$crop,}scale='min(1280,iw)':-2:flags=lanczos" -frames:v 1 "$dir/app.png" || echo "   (the picture could not be made)"
