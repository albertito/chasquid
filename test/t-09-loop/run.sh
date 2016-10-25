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

# Wait until one of them has noticed and stopped the loop.
while sleep 0.1; do
	wget -q -O .data-A/vars http://localhost:1099/debug/vars
	wget -q -O .data-B/vars http://localhost:2099/debug/vars
	if grep -q '"chasquid/smtpIn/loopsDetected": 1,' .data-?/vars; then
		break
	fi
done

success
