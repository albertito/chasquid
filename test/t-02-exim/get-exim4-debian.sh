#!/bin/bash
#
# This script downloads the exim4 binary from Debian's package.
# It assumes "apt" is functional, which means it's not very portable, but
# given the nature of these tests that's acceptable for now.

set -e
. "$(dirname "$0")/../util/lib.sh"

init

# Download and extract the package in .exim-bin
apt download exim4-daemon-light
dpkg -x exim4-daemon-light_*.deb "$PWD/.exim-bin/"

# Create a symlink to .exim4, which is the directory we will use to store
# configuration, spool, etc.
# The configuration template will look for it here.
mkdir -p .exim4
ln -sf "$PWD/.exim-bin/usr/sbin/exim4" .exim4/

# Remove the setuid bit, if there is one - we don't need it and may cause
# confusion and/or security troubles.
chmod -s .exim-bin/usr/sbin/exim4

success

