#!/bin/bash
#
# Script that is used as a Docker entrypoint.
#
# It starts minidns with a zone resolving "localhost", and overrides
# /etc/resolv.conf to use it. Then launches docker CMD.
#
# This is used for more hermetic Docker test environments.

set -e
. "$(dirname "$0")/../util/lib.sh"

init

# Go to the root of the repository.
cd ../..

# Undo the EXIT trap, so minidns continues to run in the background.
trap - EXIT

set -v

# The DNS server resolves only "localhost"; tests will rely on this, as we
# $HOSTALIASES to point our test hostnames to localhost, so it needs to
# resolve.
echo "
localhost A    127.0.0.1
localhost AAAA ::1
" > /tmp/zones

start-stop-daemon --start --background \
	--exec /tmp/minidns \
	-- --zones=/tmp/zones

echo "nameserver 127.0.0.1" > /etc/resolv.conf
echo "nameserver ::1" >> /etc/resolv.conf

# Wait until the minidns resolver comes up.
wait_until_ready 53

# Disable the Go proxy, since now there is no external network access.
# Modules should be already be made available in the environment.
export GOPROXY=off

# Launch arguments, which come from docker CMD, as "chasquid" user.
# Running tests as root makes some integration tests more difficult, as for
# example Exim has hard-coded protections against running as root.
sudo -u "chasquid" -g "chasquid" \
	--set-home \
	--preserve-env PATH="$PATH" \
	-- "$@"
