#!/bin/sh
# Entrypoint for the Iris Postfix image. It points Postfix at the routing maps
# the controller renders into the mounted ConfigMap, starts the route reloader
# in the background, then hands off to the boky/postfix entrypoint which runs
# the Postfix master in the foreground as the container's main process.
#
# boky/postfix compiles every referenced map with postmap at startup, which
# writes a database next to the source file. The ConfigMap mount is read-only,
# so the maps are first copied into a writable work directory and Postfix reads
# the compiled (lmdb) maps from there. The reloader keeps the work copy in sync
# and recompiles on change. relay_domains is a plain list, so it is referenced
# as a file whose lines Postfix substitutes in.
set -eu

src_dir="${IRIS_POSTFIX_MAPS_SRC_DIR:-/etc/postfix/maps-src}"
maps_dir="${IRIS_POSTFIX_MAPS_DIR:-/etc/postfix/maps}"

# Seed the writable work directory so the maps exist before boky compiles them.
mkdir -p "$maps_dir"
for name in transport relay_recipient_maps relay_domains; do
	if [ -f "${src_dir}/${name}" ]; then
		cp -fL "${src_dir}/${name}" "${maps_dir}/${name}"
	fi
done

postconf -e "transport_maps=lmdb:${maps_dir}/transport"
postconf -e "relay_recipient_maps=lmdb:${maps_dir}/relay_recipient_maps"
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

# Read the PROXY protocol header from an upstream L4 load balancer when one
# fronts the SMTP listeners. A plain TCP load balancer rewrites the source
# address, so without this every connection appears to originate from the load
# balancer and Postfix's client-IP-based checks (SPF, postscreen, DNSBL) and
# the maillog all see the wrong peer. The value is the upstream proxy protocol
# Postfix should expect; Postfix only implements "haproxy". Set globally so it
# applies to the smtp (25), submission (587) and smtps (465) services alike,
# which are all published through the same load balancer.
proxy_protocol="${IRIS_POSTFIX_PROXY_PROTOCOL:-}"
if [ -n "$proxy_protocol" ]; then
	postconf -e "smtpd_upstream_proxy_protocol=${proxy_protocol}"
fi

/usr/local/bin/iris-reloader &

exec /scripts/run.sh "$@"
