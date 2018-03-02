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
wget -q -o /dev/null -O .data-A/dbg-root http://localhost:1099/ \
	|| fail "failed to fetch /"
wget -q -o /dev/null -O .data-A/dbg-flags http://localhost:1099/debug/flags \
	|| fail "failed to fetch /debug/flags"
wget -q -o /dev/null -O .data-A/dbg-queue http://localhost:1099/debug/queue \
	|| fail "failed to fetch /debug/queue"
wget -q -o /dev/null -O .data-A/dbg-root http://localhost:1099/404 \
	&& fail "fetch /404 worked, should have failed"

# Wait until one of them has noticed and stopped the loop.
while sleep 0.1; do
	wget -q -o /dev/null -O .data-A/vars http://localhost:1099/debug/vars
	wget -q -o /dev/null -O .data-B/vars http://localhost:2099/debug/vars
	if grep -q '"chasquid/smtpIn/loopsDetected": 1,' .data-?/vars; then
		break
	fi
done

# Test that A has outgoing domaininfo for srv-b.
# This is unrelated to the loop itself, but serves as an end-to-end
# verification that outgoing domaininfo works.
if ! grep -q "outgoing_sec_level: TLS_INSECURE" ".data-A/domaininfo/s:srv-b";
then
	fail "A is missing the domaininfo for srv-b"
fi

success
