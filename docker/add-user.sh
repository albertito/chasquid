#!/bin/bash
#
# Creates a user. If it exists, updates the password.
#
# Note this is not robust, it's only for convenience on extremely simple
# setups.

set -e

if test -z "${EMAIL:-}"; then
        read -r -p "Email (full user@domain format): " EMAIL
fi

if ! echo "${EMAIL}" | grep -q @; then
	echo "Error: email should have '@'."
	exit 1
fi

if test -z "${PASSWORD:-}"; then
        read -r -p "Password: " -s PASSWORD
        echo
fi

DOMAIN=$(echo echo "${EMAIL}" | cut -d '@' -f 2)


# If the domain doesn't exist in chasquid's config, create it.
mkdir -p "/data/chasquid/domains/${DOMAIN}/"


# Encrypt password.
ENCPASS=$(doveadm pw -u "${EMAIL}" -p "${PASSWORD}")

# Edit dovecot users: remove user if it exits.
mkdir -p /data/dovecot
touch /data/dovecot/users
if grep -q "^${EMAIL}:" /data/dovecot/users; then
	cp /data/dovecot/users /data/dovecot/users.old
	grep -v "^${EMAIL}:" /data/dovecot/users.old \
		> /data/dovecot/users
fi

# Edit dovecot users: add user.
echo "${EMAIL}:${ENCPASS}::::" >> /data/dovecot/users

echo "${EMAIL} added to /data/dovecot/users"

