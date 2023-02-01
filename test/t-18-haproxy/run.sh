#!/bin/bash

set -e
. "$(dirname "$0")/../util/lib.sh"

init
check_hostaliases

mkdir -p .logs

if ! haproxy -v > /dev/null; then
	skip "haproxy binary not found"
fi

# Set a 2m timeout: if there are issues with haproxy, the wait tends to hang
# indefinitely, so an explicit timeout helps with test automation.
timeout 2m

# Launch haproxy in the background, checking config first to fail fast in that
# case.
haproxy -f haproxy.cfg -c
haproxy -f haproxy.cfg -d > .logs/haproxy.log 2>&1 &

generate_certs_for testserver
add_user user@testserver secretpassword
add_user someone@testserver secretpassword

chasquid -v=2 --logfile=.logs/chasquid.log --config_dir=config &

wait_until_ready 1025 # haproxy
wait_until_ready 2025 # chasquid

run_msmtp someone@testserver < content

wait_for_file .mail/someone@testserver

mail_diff content .mail/someone@testserver

success
