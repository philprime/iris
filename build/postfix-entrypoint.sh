#!/bin/sh
# Entrypoint for the Iris Postfix image. It points Postfix at the routing maps
# the controller renders into the mounted ConfigMap, starts the route reloader
# in the background, then hands off to the boky/postfix entrypoint which runs
# the Postfix master in the foreground as the container's main process.
#
# The maps are consumed as texthash, which Postfix reads directly from the
# read-only ConfigMap mount. No postmap compilation is needed (and it could not
# write a .db next to a read-only file anyway). The reloader runs postfix reload
# on change so the daemons re-read the maps. relay_domains is a plain list, so
# it is referenced as a file whose lines Postfix substitutes in.
set -eu

maps_dir="${IRIS_POSTFIX_MAPS_DIR:-/etc/postfix/maps}"

postconf -e "transport_maps=texthash:${maps_dir}/transport"
postconf -e "relay_recipient_maps=texthash:${maps_dir}/relay_recipient_maps"
postconf -e "relay_domains=${maps_dir}/relay_domains"

/usr/local/bin/iris-reloader &

exec /scripts/run.sh "$@"
