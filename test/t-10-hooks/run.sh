#!/bin/bash

set -e
. $(dirname ${0})/../util/lib.sh

init

generate_certs_for testserver
add_user user@testserver secretpassword
add_user someone@testserver secretpassword
add_user blockme@testserver secretpassword
add_user permanent@testserver secretpassword

mkdir -p .logs
chasquid -v=2 --logfile=.logs/chasquid.log --config_dir=config &
wait_until_ready 1025

cp config/hooks/post-data.good config/hooks/post-data

run_msmtp someone@testserver < content

wait_for_file .mail/someone@testserver

mail_diff content .mail/someone@testserver

if ! grep -q "X-Post-Data: success" .mail/someone@testserver; then
	fail "missing X-Post-Data header"
fi

function check() {
	if ! grep -q "$1" .data/post-data.out; then
		fail "missing: $1"
	fi
}

# Verify that the environment for the hook was reasonable.
check "RCPT_TO=someone@testserver"
check "MAIL_FROM=user@testserver"
check "USER=$USER"
check "PWD=$PWD/config"
check "EHLO_DOMAIN=localhost"
check "EHLO_DOMAIN_RAW=localhost"
check "FROM_LOCAL_DOMAIN=1"
check "ON_TLS=1"
check "AUTH_AS=user@testserver"
check "PATH="
check "REMOTE_ADDR="
check "SPF_PASS=0"


# Check that failures in the script result in failing delivery.
# Transient failure.
if run_msmtp blockme@testserver < content 2>/dev/null; then
	fail "ERROR: hook did not block email as expected"
fi
if ! tail -n 1 .logs/msmtp | grep -q "smtpstatus=451"; then
	tail -n 1 .logs/msmtp
	fail "ERROR: transient hook error not returned correctly"
fi

# Permanent failure.
if run_msmtp permanent@testserver < content 2>/dev/null; then
	fail "ERROR: hook did not block email as expected"
fi
if ! tail -n 1 .logs/msmtp | grep -q "smtpstatus=554"; then
	tail -n 1 .logs/msmtp
	fail "ERROR: permanent hook error not returned correctly"
fi

# Check that the bad hooks don't prevent delivery.
for i in config/hooks/post-data.bad*; do
	cp $i config/hooks/post-data

	run_msmtp someone@testserver < content
	wait_for_file .mail/someone@testserver
	mail_diff content .mail/someone@testserver
done

success
