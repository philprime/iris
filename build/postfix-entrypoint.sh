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

# Enable opportunistic STARTTLS when a serving certificate is mounted. The
# secret cert-manager fills holds tls.crt and tls.key. Opportunistic level
# "may" keeps plaintext working for senders that do not offer TLS. The
# submission (587) and smtps (465) services are defined so the exposed ports
# serve TLS too; 465 uses wrapper mode (implicit TLS).
tls_dir="${IRIS_POSTFIX_TLS_DIR:-}"
if [ -n "$tls_dir" ] && [ -f "${tls_dir}/tls.crt" ] && [ -f "${tls_dir}/tls.key" ]; then
	postconf -e "smtpd_tls_cert_file=${tls_dir}/tls.crt"
	postconf -e "smtpd_tls_key_file=${tls_dir}/tls.key"
	postconf -e "smtpd_tls_security_level=may"
	postconf -e "smtpd_tls_loglevel=1"

	postconf -M "submission/inet=submission inet n - n - - smtpd"
	postconf -P "submission/inet/smtpd_tls_security_level=may"

	postconf -M "smtps/inet=smtps inet n - n - - smtpd"
	postconf -P "smtps/inet/smtpd_tls_wrappermode=yes"
	postconf -P "smtps/inet/smtpd_tls_security_level=encrypt"
fi

/usr/local/bin/iris-reloader &

exec /scripts/run.sh "$@"
