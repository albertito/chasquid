#!/bin/bash

set -e
. "$(dirname "$0")/../util/lib.sh"

init

generate_certs_for testserver
add_user user@testserver secretpassword

# Build in advance, so the build doesn't run under the low fd limit below.
go-build-cached "${TBASE}/../../"

# Run chasquid with a low file descriptor limit, which is easy to exhaust.
# Reload very often: reloading needs to open files, so if we ran out of file
# descriptors, chasquid would fail to reload and exit.
mkdir -p .logs
(
	ulimit -n 64
	chasquid -v=2 --logfile=.logs/chasquid.log --config_dir=config \
		--testing__reload_every=50ms
) &
CHASQUID_PID=$!
wait_until_ready 1025

# The connection limit should be half the file descriptor limit.
if ! grep -q "Incoming connection limit: 32$" .logs/chasquid.log; then
	fail "unexpected connection limit"
fi

# Open and hold as many connections as the limit allows.
conngen -addr=localhost:1025 -count=32 -wait > .logs/conngen.log 2>&1 &
CONNGEN_PID=$!
wait_until "grep -q 'done, created' .logs/conngen.log"

# Open and hold more connections than there are file descriptors left. The
# server should close them right away (so we read EOF, not time out).
for _ in $(seq 1 40); do
	exec {fd}<>/dev/tcp/localhost/1025
	if read -r -t 5 line <&"$fd"; then
		fail "connection over the limit got: $line"
	elif [ $? -gt 128 ]; then
		fail "connection over the limit was not closed"
	fi
done

if ! grep -q "Incoming connection limit (32) reached" .logs/chasquid.log; then
	fail "connection limit not logged"
fi

# Give it time to reload a few times, and check it's still running.
sleep 0.5
if ! kill -0 $CHASQUID_PID; then
	fail "chasquid is not running"
fi

# Once the connections are closed, it should accept new ones, and work
# normally. The server notices the connections were closed asynchronously, so
# we retry until it works.
kill $CONNGEN_PID
wait_until "smtpc user@testserver < content 2>/dev/null"
wait_for_file .mail/user@testserver

success
