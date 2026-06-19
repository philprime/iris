#!/bin/sh
# Entrypoint for the Iris Postfix image. It starts the route reloader in the
# background, then hands off to the boky/postfix entrypoint which runs the
# Postfix master in the foreground as the container's main process. The
# reloader watches the mounted maps and runs postmap plus postfix reload on
# change.
set -eu

/usr/local/bin/iris-reloader &

exec /scripts/run.sh "$@"
