#!/bin/bash

set -e
. $(dirname ${0})/../util/lib.sh

init

generate_certs_for testserver

# Start with the user with the wrong password, and no aliases.
add_user someone@testserver password111
rm -f config/domains/testserver/aliases

mkdir -p .logs
chasquid -v=2 --logfile=.logs/chasquid.log --config_dir=config \
	--testing__reload_every=50ms &
wait_until_ready 1025

# First, check that delivery fails with the "wrong" password.
if run_msmtp someone@testserver < content 2>/dev/null; then
	fail "success using the wrong password"
fi

# Change password, add an alias; then wait a bit more than the reload period
# and try again.
add_user someone@testserver password222
echo "analias: someone" > config/domains/testserver/aliases
sleep 0.2

run_msmtp analias@testserver < content
wait_for_file .mail/someone@testserver

success
