#!/bin/bash

set -e
. $(dirname ${0})/../util/lib.sh

init

generate_certs_for testserver
add_user testserver user secretpassword
add_user testserver someone secretpassword

mkdir -p .logs
chasquid -v=2 --log_dir=.logs --config_dir=config &
wait_until_ready 1025

run_msmtp someone@testserver < content

wait_for_file .mail/someone@testserver

mail_diff content .mail/someone@testserver

# At least for now, we allow AUTH over the SMTP port to avoid unnecessary
# complexity, so we expect it to work.
if ! run_msmtp -a smtpport someone@testserver < content 2> /dev/null; then
	echo "ERROR: failed auth on the SMTP port"
	exit 1
fi

if run_msmtp nobody@testserver < content 2> /dev/null; then
	echo "ERROR: successfuly sent an email to a non-existent user"
	exit 1
fi

if run_msmtp -a baduser someone@testserver < content 2> /dev/null; then
	echo "ERROR: successfully sent an email with a bad password"
	exit 1
fi

if run_msmtp -a badpasswd someone@testserver < content 2> /dev/null; then
	echo "ERROR: successfully sent an email with a bad password"
	exit 1
fi

success
