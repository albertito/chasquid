#!/bin/bash

set -e
. $(dirname ${0})/../util/lib.sh

init

generate_certs_for testserver
add_user user@testserver secretpassword

# Note we run the server with minimal logging, to avoid generating very large
# log files, which are not very useful anyway.
mkdir -p .logs
chasquid -v=-1 --logfile=.logs/chasquid.log --config_dir=config &
wait_until_ready 1025

echo Peak RAM: `chasquid_ram_peak`

# Set connection count to (max open files) - (leeway).
# We set the leeway to account for file descriptors opened by the runtime and
# listeners; 20 should be enough for now.
# Cap it to 2000, as otherwise it can be problematic due to port availability.
COUNT=$(( `ulimit -n` - 20 ))
if [ $COUNT -gt 2000 ]; then
	COUNT=2000
fi

if ! conngen -logtime -addr=localhost:1025 -count=$COUNT; then
	tail -n 1 .logs/chasquid.log
	fail
fi

echo Peak RAM: `chasquid_ram_peak`

success
