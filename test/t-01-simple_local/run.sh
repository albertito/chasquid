#!/bin/bash

set -e
. $(dirname ${0})/../util/lib.sh

init

generate_certs_for testserver

chasquid -v=2 --log_dir=.logs --config_dir=config &
wait_until_ready 1025

run_msmtp someone@testserver < content

wait_for_file .mail/someone@testserver

mail_diff content .mail/someone@testserver

if run_msmtp -a baduser someone@testserver < content 2> /dev/null; then
	echo "ERROR: successfully sent an email with a bad password"
	exit 1
fi

if run_msmtp -a badpasswd someone@testserver < content 2> /dev/null; then
	echo "ERROR: successfully sent an email with a bad password"
	exit 1
fi

success
