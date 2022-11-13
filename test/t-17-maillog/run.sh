#!/bin/bash

set -e
. "$(dirname "$0")/../util/lib.sh"

init
check_hostaliases

mkdir -p .logs

generate_certs_for testserver
add_user user@testserver secretpassword
add_user someone@testserver secretpassword

function send_one() {
	rm -f .logs/mail_log .logs/stdout .logs/stderr
	envsubst < config/chasquid.conf.in > config/chasquid.conf

	chasquid -v=2 --logfile=.logs/chasquid.log --config_dir=config \
		> .logs/stdout 2> .logs/stderr &
	wait_until_ready 1025

	run_msmtp someone@testserver < content
	wait_for_file .mail/someone@testserver
	mail_diff content .mail/someone@testserver

	pkill -s 0 chasquid
	sleep 0.2
}

export MAIL_LOG_PATH="../.logs/mail_log"
send_one
if ! grep -q "from=user@testserver all done" .logs/mail_log; then
	fail "entries not found in .logs/mail_log"
fi

export MAIL_LOG_PATH="<stdout>"
send_one
if ! grep -q "from=user@testserver all done" .logs/stdout; then
	fail "entries not found in .logs/stdout"
fi

export MAIL_LOG_PATH="<stderr>"
send_one
if ! grep -q "from=user@testserver all done" .logs/stderr; then
	fail "entries not found in .logs/stderr"
fi

success
