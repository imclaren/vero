# GTK4 and a virtual display, so a GTK app - the example, or yours - can
# run, and be watched, on a Mac. Built by scripts/run-linux.sh, which can
# choose another Debian (BASE) and add packages (EXTRA): WebKitGTK, say.
ARG BASE=debian:bookworm-slim
FROM ${BASE}
ARG EXTRA=""
RUN apt-get update && apt-get install -y --no-install-recommends \
      python3 python3-gi gir1.2-gtk-4.0 libgtk-4-1 \
      xvfb x11vnc x11-utils imagemagick xauth ca-certificates adwaita-icon-theme \
      ${EXTRA} \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
