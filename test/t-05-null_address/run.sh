#!/bin/bash

set -e
. $(dirname ${0})/../util/lib.sh

init

generate_certs_for testserver
add_user user@testserver secretpassword

mkdir -p .logs
chasquid -v=2 --logfile=.logs/chasquid.log --config_dir=config &
wait_until_ready 1025


# Send mail with an empty address (directly, unauthenticated).
nc localhost 1025 < sendmail > /dev/null
wait_for_file .mail/user@testserver
mail_diff content .mail/user@testserver
rm -f .mail/user@testserver


# Test that we get mail back for a failed delivery
run_msmtp fail@testserver < content
wait_for_file .mail/user@testserver
mail_diff expected_dsr .mail/user@testserver


success
