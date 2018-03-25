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

if ! loadgen -logtime -addr=localhost:1025 -run_for=3s -noop; then
	fail
fi

echo Peak RAM: `chasquid_ram_peak`

if ! loadgen -logtime -addr=localhost:1025 -run_for=3s; then
	fail
fi

echo Peak RAM: `chasquid_ram_peak`

success
