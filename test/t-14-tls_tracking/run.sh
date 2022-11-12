#!/bin/bash

# Test TLS tracking features, which require faking SPF.

set -e
. $(dirname ${0})/../util/lib.sh

init
check_hostaliases

# Build with the DNS override, so we can fake DNS records.
export GOTAGS="dnsoverride"

# Launch minidns in the background using our configuration.
minidns_bg --addr=":9053" -zones=zones >> .minidns.log 2>&1


# Two chasquid servers:
# A - listens on :1025, hosts srv-A
# B - listens on :2025, hosts srv-B

CONFDIR=A generate_certs_for srv-A
CONFDIR=A add_user usera@srv-A userA

CONFDIR=B generate_certs_for srv-B
CONFDIR=B add_user userb@srv-B userB

rm -rf .data-A .data-B .mail .certs
mkdir -p .logs-A .logs-B .mail .certs

# Put public certs in .certs, and use it as our trusted cert dir.
cp A/certs/srv-A/fullchain.pem .certs/srv-a.pem
cp B/certs/srv-B/fullchain.pem .certs/srv-b.pem
export SSL_CERT_DIR=$PWD/.certs/

chasquid -v=2 --logfile=.logs-A/chasquid.log --config_dir=A \
	--testing__dns_addr=127.0.0.1:9053 \
	--testing__max_received_headers=5 \
	--testing__outgoing_smtp_port=2025 &
chasquid -v=2 --logfile=.logs-B/chasquid.log --config_dir=B \
	--testing__dns_addr=127.0.0.1:9053 \
	--testing__outgoing_smtp_port=1025 &

wait_until_ready 1025
wait_until_ready 2025
wait_until_ready 9053

run_msmtp userB@srv-B < content

wait_for_file .mail/userb@srv-b
mail_diff content .mail/userb@srv-b

# A should have a secure outgoing connection to srv-b.
if ! grep -q 'outgoing_sec_level:\s*TLS_SECURE' ".data-A/domaininfo/s:srv-b";
then
	fail "A is missing the domaininfo for srv-b"
fi

# B should have a secure incoming connection from srv-a.
if ! grep -q 'incoming_sec_level:\s*TLS_CLIENT' ".data-B/domaininfo/s:srv-a";
then
	fail "B is missing the domaininfo for srv-a"
fi

success

