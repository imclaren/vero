#!/bin/sh
# Builds the worker. Run from this directory.
#
# There is no shared library to build: the worker supervises itself, and the
# Python binding spawns it rather than loading one.
set -e
cd "$(dirname "$0")"

echo "building the worker"
go build -o worker ../worker

echo
echo "run it with:  ./main.py"
echo "needs:        python3-gi and gir1.2-gtk-4.0"
