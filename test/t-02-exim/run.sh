#!/bin/bash
#
# This test checks that we can send and receive mail to/from exim4.
#
# Setup:
#   - chasquid listening on :1025.
#   - exim listening on :2025.
#   - hosts "srv-chasquid" and "srv-exim" pointing back to localhost.
#   - exim configured to accept all email and forward it to
#     someone@srv-chasquid.
#
# Test:
#   msmtp --> chasquid --> exim --> chasquid --> local delivery
#
#   msmtp will auth as user@srv-chasquid to chasquid, and send an email with
#   recipient someone@srv-exim.
#
#   chasquid will deliver the mail to exim.
#
#   exim will deliver the mail back to chasquid (after changing the
#   destination to someone@chasquid).
#
#   chasquid will receive the email from exim, and deliver it locally.

set -e
. $(dirname ${0})/../util/lib.sh

init

# Create a temporary directory for exim4 to use, and generate the exim4
# config based on the template.
mkdir -p .exim4
EXIMDIR="$PWD/.exim4" envsubst < config/exim4.in > .exim4/config

generate_certs_for srv-chasquid
add_user srv-chasquid user secretpassword

# Launch chasquid at port 1025 (in config).
# Use outgoing port 2025 which is where exim will be at.
# Bypass MX lookup, so it can find srv-exim (via our host alias).
mkdir -p .logs
chasquid -v=2 --log_dir=.logs --config_dir=config \
	--testing__outgoing_smtp_port=2025 \
	--testing__bypass_mx_lookup &

wait_until_ready 1025

# Launch exim at port 2025
.exim4/exim4 -bd -d -C "$PWD/.exim4/config" > .exim4/log 2>&1 &
wait_until_ready 2025

# msmtp will use chasquid to send an email to someone@srv-exim.
run_msmtp someone@srv-exim < content

wait_for_file .mail/someone@srv-chasquid

mail_diff content .mail/someone@srv-chasquid

success
