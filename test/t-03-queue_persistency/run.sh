#!/bin/bash

set -e
. "$(dirname "$0")/../util/lib.sh"

init

# Don't hang forever if queue items are not processed as expected.
timeout 1m

# Start with an empty queue and mail log, so leftovers from previous runs
# don't interfere.
rm -rf .data/queue .logs/mail_log

# Add an item to the queue before starting chasquid.
go run addtoqueue.go --queue_dir=.data/queue \
		--from someone@testserver \
		--rcpt someone@testserver \
		< content

# Add items with invalid domains, which older versions could have queued.
# They should fail delivery, but not crash the daemon.
# A DSN to an invalid domain.
go run addtoqueue.go --queue_dir=.data/queue --id=invalid1 \
		--from "<>" \
		--rcpt "x@evil/foo" \
		< content
# A message from an invalid domain, which fails delivery and so generates a
# DSN to the invalid domain.
go run addtoqueue.go --queue_dir=.data/queue --id=invalid2 \
		--from "x@evil/foo" \
		--rcpt_type PIPE --rcpt "false" \
		< content

generate_certs_for testserver

mkdir -p .logs
chasquid -v=2 --logfile=.logs/chasquid.log --config_dir=config &
CHASQUID_PID=$!

# Wait for all the queue items to be processed (or chasquid to exit), and
# check that chasquid is still running.
function queue_empty_or_exited() {
	! kill -0 $CHASQUID_PID 2>/dev/null || \
		! compgen -G '.data/queue/m:*' > /dev/null
}
wait_until queue_empty_or_exited
if ! kill -0 $CHASQUID_PID 2>/dev/null; then
	fail "chasquid exited unexpectedly"
fi

# Check that the valid item in the queue was delivered.
mail_diff content .mail/someone@testserver

# Check that the DSNs to the invalid domain failed as expected (one queued
# directly, one generated from the failed delivery).
if [ "$(grep -c 'from=<> to=x@evil/foo failed (permanent)' \
		.logs/mail_log)" != 2 ]; then
	fail "did not find the expected DSN failures in the mail log"
fi

success
