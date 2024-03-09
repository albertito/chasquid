#!/bin/bash

# Test SPF resolution, which requires overriding DNS server.
# Note this aims at providing some general end to end coverage, as well as the
# main gaps.

set -e
. "$(dirname "$0")/../util/lib.sh"

init
check_hostaliases

# Build with the DNS override, so we can fake DNS records.
export GOTAGS="dnsoverride"

# Two chasquid servers:
# A - listens on :1025, hosts srv-A
# B - listens on :2025, hosts srv-B

CONFDIR=A generate_certs_for srv-A
CONFDIR=A add_user usera@srv-A userA

CONFDIR=B generate_certs_for srv-B
CONFDIR=B add_user userb@srv-B userB

rm -rf .data-A .data-B .mail .certs
mkdir -p .logs-A .logs-B .mail .certs

# Put public certs in .certs, and use it as our trusted cert dir.
cp A/certs/srv-A/fullchain.pem .certs/srv-a.pem
cp B/certs/srv-B/fullchain.pem .certs/srv-b.pem
export SSL_CERT_DIR=$PWD/.certs/

chasquid -v=2 --logfile=.logs-A/chasquid.log --config_dir=A \
	--testing__dns_addr=127.0.0.1:9053 \
	--testing__max_received_headers=5 \
	--testing__outgoing_smtp_port=2025 &
chasquid -v=2 --logfile=.logs-B/chasquid.log --config_dir=B \
	--testing__dns_addr=127.0.0.1:9053 \
	--testing__outgoing_smtp_port=1025 &

wait_until_ready 1025
wait_until_ready 2025

function launch_minidns() {
	if [ "$MINIDNS" != "" ]; then
		kill "$MINIDNS"
		wait "$MINIDNS" || true
	fi
	cp "$1" .zones
	minidns_bg --addr=":9053" -zones=.zones >> .minidns.log 2>&1
	wait_until_ready 9053
}

# T0: Successful.
launch_minidns zones.t0
smtpc userB@srv-B < content
wait_for_file .mail/userb@srv-b
mail_diff content .mail/userb@srv-b

# T1: A is not permitted to send to B.
# Check that userA got a DSN about it.
rm .mail/*
launch_minidns zones.t1
smtpc userB@srv-B < content
wait_for_file .mail/usera@srv-a
mail_diff expected_dsn .mail/usera@srv-a

success
