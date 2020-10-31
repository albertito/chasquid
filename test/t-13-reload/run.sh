#!/bin/bash

set -e
. $(dirname ${0})/../util/lib.sh

init

generate_certs_for testserver

#
# Automatic reload.
#

# Start with the user with the wrong password, and no aliases.
add_user someone@testserver password111
rm -f config/domains/testserver/aliases

mkdir -p .logs
chasquid -v=2 --logfile=.logs/chasquid.log --config_dir=config \
	--testing__reload_every=50ms &
wait_until_ready 1025

# First, check that delivery fails with the "wrong" password.
if run_msmtp someone@testserver < content 2>/dev/null; then
	fail "success using the wrong password"
fi

# Change password, add an alias; then wait a bit more than the reload period
# and try again.
add_user someone@testserver password222
echo "analias: someone" > config/domains/testserver/aliases
sleep 0.2

run_msmtp analias@testserver < content
wait_for_file .mail/someone@testserver


#
# Manual log rotation.
#

# Rotate logs.
mv .logs/chasquid.log .logs/chasquid.log-old
mv .logs/mail_log .logs/mail_log-old

# Send SIGHUP and give it a little for the server to handle it.
pkill -HUP -s 0 chasquid
sleep 0.2

# Send another mail.
rm .mail/someone@testserver
run_msmtp analias@testserver < content
wait_for_file .mail/someone@testserver

# Check there are new entries.
sleep 0.2
if ! grep -q "from=someone@testserver all done" .logs/mail_log; then
	fail "new mail log did not have the expected entry"
fi
if ! grep -q -E "Queue.SendLoop .*: someone@testserver sent" .logs/chasquid.log;
then
	fail "new chasquid log did not have the expected entry"
fi


# Test that we can make the server exit using the /exit endpoint.
# First, a GET should fail with status 405.
fexp http://localhost:1099/exit -status 405

# A POST should succeed, return an OK body, and the daemon should
# eventually exit.
CHASQUID_PID=$(pgrep -s 0 chasquid)
fexp http://localhost:1099/exit -method POST -bodyre "OK"
wait_until ! kill -s 0 $CHASQUID_PID 2> /dev/null

success
