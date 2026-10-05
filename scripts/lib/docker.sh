# Sourced by the scripts that use Docker.
#
# vero_docker checks that Docker is running, and works round a Mac that once
# had Docker Desktop: its ~/.docker/config.json keeps "credsStore":
# "desktop" after Docker Desktop is gone, and every pull then fails looking
# for docker-credential-desktop. Such a config is copied without it, with
# its contexts (colima's among them), and used instead.
vero_docker() {
    docker info >/dev/null 2>&1 || { echo "docker is not running - try: colima start" >&2; exit 1; }
    cfg=${DOCKER_CONFIG:-$HOME/.docker}
    store=$(sed -n 's/.*"credsStore"[^"]*"\([^"]*\)".*/\1/p' "$cfg/config.json" 2>/dev/null)
    if [ -n "$store" ] && ! command -v "docker-credential-$store" >/dev/null 2>&1; then
        clean="$HOME/.cache/vero-docker"
        mkdir -p "$clean"
        cp -R "$cfg/contexts" "$clean/" 2>/dev/null || true
        python3 -c 'import json, sys
c = json.load(open(sys.argv[1]))
c.pop("credsStore", None); c.pop("credHelpers", None)
json.dump(c, open(sys.argv[2], "w"))' "$cfg/config.json" "$clean/config.json"
        export DOCKER_CONFIG="$clean"
    fi
}
