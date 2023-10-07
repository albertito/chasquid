#!/bin/bash
#
# This test checks that we can use dovecot as an authentication mechanism.
#
# Setup:
#  - chasquid listening on :1025.
#  - dovecot listening on unix sockets in .dovecot/

set -e
. "$(dirname "$0")/../util/lib.sh"

init
check_hostaliases

if ! dovecot --version > /dev/null; then
	skip "dovecot not installed"
fi

# Create a temporary directory for dovecot to use, and generate the dovecot
# config based on the template.
# Note the length of the path must be < 100, because unix sockets have a low
# limitation, so we use a directory in /tmp, which is not ideal, as a
# workaround.
export ROOT="/tmp/chasquid-dovecot-test"
mkdir -p $ROOT $ROOT/run $ROOT/lib
rm -f $ROOT/dovecot.log

GROUP=$(id -g -n) envsubst \
	< config/dovecot.conf.in > $ROOT/dovecot.conf
cp -f config/passwd $ROOT/passwd

dovecot -F -c $ROOT/dovecot.conf &

# Early tests: run dovecot-auth-cli for testing purposes. These fail early if
# there are obvious problems.
OUT=$(dovecot-auth-cli $ROOT/run/auth exists user@srv || true)
if [ "$OUT" != "yes" ]; then
	fail "user does not exist: $OUT"
fi

OUT=$(dovecot-auth-cli $ROOT/run/auth auth user@srv password || true)
if [ "$OUT" != "yes" ]; then
	fail "auth failed: $OUT"
fi


# Set up chasquid, using dovecot as authentication backend.
generate_certs_for srv

mkdir -p .logs
chasquid -v=2 --logfile=.logs/chasquid.log --config_dir=config &
wait_until_ready 1025

# Send an email as "user@srv" successfully.
run_msmtp user@srv < content
wait_for_file .mail/user@srv
mail_diff content .mail/user@srv

# Send an email as "naked" successfully.
rm .mail/user@srv
run_msmtp -a naked user@srv < content
wait_for_file .mail/user@srv
mail_diff content .mail/user@srv

# Send an email to the "naked" user successfully.
run_msmtp naked@srv < content
wait_for_file .mail/naked@srv
mail_diff content .mail/naked@srv

# Fail to send to nobody@srv (user does not exist).
if run_msmtp nobody@srv < content 2> /dev/null; then
	fail "successfully sent an email to a non-existent user"
fi

# Fail to send from baduser@srv (user does not exist).
if run_msmtp -a baduser user@srv < content 2> /dev/null; then
	fail "successfully sent an email with a bad user"
fi

# Fail to send with an incorrect password.
if run_msmtp -a badpasswd user@srv < content 2> /dev/null; then
	fail "successfully sent an email with a bad password"
fi

success
