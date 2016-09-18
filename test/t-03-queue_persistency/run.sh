#!/bin/bash

set -e
. $(dirname ${0})/../util/lib.sh

init

# Add an item to the queue before starting chasquid.
go run addtoqueue.go --queue_dir=.data/queue \
		--from someone@testserver \
		--rcpt someone@testserver \
		< content

generate_certs_for testserver

mkdir -p .logs
chasquid -v=2 --log_dir=.logs --config_dir=config &
wait_until_ready 1025

# Check that the item in the queue was delivered.
wait_for_file .mail/someone@testserver

mail_diff content .mail/someone@testserver

success
