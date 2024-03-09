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
#   smtpc --> chasquid --> exim --> chasquid --> local delivery
#
#   smtpc will auth as user@srv-chasquid to chasquid, and send an email with
#   recipient someone@srv-exim.
#
#   chasquid will deliver the mail to exim.
#
#   exim will deliver the mail back to chasquid (after changing the
#   destination to someone@chasquid).
#
#   chasquid will receive the email from exim, and deliver it locally.

set -e
. "$(dirname "$0")/../util/lib.sh"

init
check_hostaliases

# Create a temporary directory for exim4 to use, and generate the exim4
# config based on the template.
mkdir -p .exim4
EXIMDIR="$PWD/.exim4" envsubst < config/exim4.in > .exim4/config

if ! .exim4/exim4 -C "$PWD/.exim4/config" --version > /dev/null; then
	skip "exim4 binary at .exim4/exim4 is not functional"
fi

# Build with the DNS override, so we can fake DNS records.
export GOTAGS="dnsoverride"

# Launch minidns in the background using our configuration.
minidns_bg --addr=":9053" -zones=zones >> .minidns.log 2>&1

generate_certs_for srv-chasquid
add_user user@srv-chasquid secretpassword
add_user someone@srv-chasquid secretpassword

# Launch chasquid at port 1025 (in config).
# Use outgoing port 2025 which is where exim will be at.
# Bypass MX lookup, so it can find srv-exim (via our host alias).
mkdir -p .logs
chasquid -v=2 --logfile=.logs/chasquid.log --config_dir=config \
	--testing__dns_addr=127.0.0.1:9053 \
	--testing__outgoing_smtp_port=2025 &

wait_until_ready 1025
wait_until_ready 9053

# Launch exim at port 2025
.exim4/exim4 -bd -d -C "$PWD/.exim4/config" > .exim4/log 2>&1 &
wait_until_ready 2025

# smtpc will use chasquid to send an email to someone@srv-exim.
smtpc someone@srv-exim < content

wait_for_file .mail/someone@srv-chasquid

mail_diff content .mail/someone@srv-chasquid

success
