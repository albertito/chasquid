#!/bin/bash

set -e
. "$(dirname "$0")/../util/lib.sh"

init
check_hostaliases

rm -rf .data-A .data-B .mail

skip_if_python_is_too_old

# Build with the DNS override, so we can fake DNS records.
export GOTAGS="dnsoverride"

# srv-A has a pre-generated key, and the mail has a pre-generated header.
# Generate a key for srv-B, and append it to our statically configured zones.
# Use a fixed selector so we can be more thorough in from_B_to_A.expected.
rm -f B/domains/srv-b/*.pem
mkdir -p B/domains/srv-b/
CONFDIR=B chasquid-util dkim-keygen srv-b sel77 --algo=ed25519 > /dev/null

cp zones .zones
CONFDIR=B chasquid-util dkim-dns srv-b | sed 's/"//g' >> .zones

# Launch minidns in the background using our configuration.
minidns_bg --addr=":9053" -zones=.zones >> .minidns.log 2>&1

# Two servers:
# A - listens on :1025, hosts srv-A
# B - listens on :2015, hosts srv-B

CONFDIR=A generate_certs_for srv-A
CONFDIR=A add_user user-a@srv-a nadaA

CONFDIR=B generate_certs_for srv-B
CONFDIR=B add_user user-b@srv-b nadaB

mkdir -p .logs-A .logs-B

chasquid -v=2 --logfile=.logs-A/chasquid.log --config_dir=A \
	--testing__dns_addr=127.0.0.1:9053 \
	--testing__outgoing_smtp_port=2025 &
chasquid -v=2 --logfile=.logs-B/chasquid.log --config_dir=B \
	--testing__dns_addr=127.0.0.1:9053 \
	--testing__outgoing_smtp_port=1025 &

wait_until_ready 1025
wait_until_ready 2025
wait_until_ready 9053

# Send from A to B.
smtpc.py --server=localhost:1025 --user=user-a@srv-a --password=nadaA \
	< from_A_to_B

wait_for_file .mail/user-b@srv-b
mail_diff from_A_to_B.expected .mail/user-b@srv-b

# Send from B to A.
smtpc.py --server=localhost:2025 --user=user-b@srv-b --password=nadaB \
	< from_B_to_A

wait_for_file .mail/user-a@srv-a
mail_diff from_B_to_A.expected .mail/user-a@srv-a


success
