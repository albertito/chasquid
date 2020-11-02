#!/bin/bash

set -e
. $(dirname ${0})/../util/lib.sh

init

rm -rf .data-A .data-B .mail

# Two servers:
# A - listens on :1025, hosts srv-A
# B - listens on :2015, hosts srv-B
#
# We cause the following loop:
#   userA -> aliasB -> aliasA -> aliasB -> ...

CONFDIR=A generate_certs_for srv-A
CONFDIR=A add_user userA@srv-A userA

CONFDIR=B generate_certs_for srv-B

mkdir -p .logs-A .logs-B

chasquid -v=2 --logfile=.logs-A/chasquid.log --config_dir=A \
	--testing__max_received_headers=5 \
	--testing__outgoing_smtp_port=2025 &
chasquid -v=2 --logfile=.logs-B/chasquid.log --config_dir=B \
	--testing__outgoing_smtp_port=1025 &

wait_until_ready 1025
wait_until_ready 2025

run_msmtp aliasB@srv-B < content

# Get some of the debugging pages, for troubleshooting, and to make sure they
# work reasonably well.
function fexp_gt10() {
	fexp $1 -save $2 && \
		[ $( cat $2 | wc -l ) -gt 10 ]
}

fexp_gt10 http://localhost:1099/ .data-A/dbg-root \
	|| fail "failed to fetch /"
fexp_gt10 http://localhost:1099/debug/flags .data-A/dbg-flags \
	|| fail "failed to fetch /debug/flags"
fexp http://localhost:1099/debug/queue -save .data-A/dbg-queue \
	|| fail "failed to fetch /debug/queue"
fexp_gt10 http://localhost:1099/debug/config .data-A/dbg-config \
	|| fail "failed to fetch /debug/config"
fexp http://localhost:1099/404 -status 404 \
	|| fail "fetch /404 worked, should have failed"
fexp_gt10 http://localhost:1099/metrics .data-A/metrics \
	|| fail "failed to fetch /metrics"

# Quick sanity-check of the /metrics page, just in case.
grep -q '^chasquid_queue_itemsWritten [0-9]\+$' .data-A/metrics \
	|| fail "A /metrics is missing the chasquid_queue_itemsWritten counter"

# Wait until one of them has noticed and stopped the loop.
while sleep 0.1; do
	fexp http://localhost:1099/debug/vars -save .data-A/vars
	fexp http://localhost:2099/debug/vars -save .data-B/vars
	# Allow for up to 2 loops to be detected, because if chasquid is fast
	# enough the DSN will also loop before this check notices it.
	if grep -q '"chasquid/smtpIn/loopsDetected": [12],' .data-?/vars; then
		break
	fi
done

# Test that A has outgoing domaininfo for srv-b.
# This is unrelated to the loop itself, but serves as an end-to-end
# verification that outgoing domaininfo works.
if ! grep -q 'outgoing_sec_level:\s*TLS_INSECURE' ".data-A/domaininfo/s:srv-b";
then
	fail "A is missing the domaininfo for srv-b"
fi

success
