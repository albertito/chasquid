#!/bin/bash
#
# Script that is used as a Docker entrypoint.
#

set -e

if ! grep -q data /proc/mounts; then
	echo "/data is not mounted."
	echo "Check that the /data volume is set up correctly."
	exit 1
fi

# Create the directory structure if it's not there.
# Some of these directories are symlink targets, see the Dockerfile.
mkdir -p /data/chasquid
mkdir -p /data/letsencrypt
mkdir -p /data/chasquid
mkdir -p /data/chasquid/domains
mkdir -p /data/dovecot

# Set up the certificates for the requested domains.
if [ "$AUTO_CERTS" != "" ]; then
	# If we were given an email to use for letsencrypt, use it. Otherwise
	# continue without one.
	MAIL_OPTS="--register-unsafely-without-email"
	if [ "$CERTS_MAIL" != "" ]; then
		MAIL_OPTS="-m $CERTS_MAIL"
	fi

	for DOMAIN in $(echo $AUTO_CERTS); do
		# If it has never been set up, then do so.
		if ! [ -e /etc/letsencrypt/live/$DOMAIN/fullchain.pem ]; then
			certbot certonly \
				--non-interactive \
				--standalone \
				--agree-tos \
				$MAIL_OPTS \
				-d $DOMAIN
		else
			echo "$DOMAIN certificate already set up."
		fi
	done

	# Renew on startup, since the container won't have cron facilities.
	# Note this requires you to restart every week or so, to make sure
	# your certificate does not expire.
	certbot renew

	# Give chasquid access to the certificates.
	# Dovecot does not need this as it reads them as root.
	setfacl -R -m u:chasquid:rX /etc/letsencrypt/{live,archive}
fi

CERT_DOMAINS=""
for i in $(ls /etc/letsencrypt/live/); do
	if [ -e "/etc/letsencrypt/live/$i/fullchain.pem" ]; then
		CERT_DOMAINS="$CERT_DOMAINS $i"
	fi
done

# We need one domain to use as a default - pick the last one.
ONE_DOMAIN=$i

# Check that there's at least once certificate at this point.
if [ "$CERT_DOMAINS" == "" ]; then
	echo "No certificates found."
	echo
	echo "Set AUTO_CERTS='example.com' to automatically get one."
	exit 1
fi

# Give chasquid access to the data directory.
mkdir -p /data/chasquid/data
chown -R chasquid /data/chasquid/

# Give dovecot access to the mailbox home.
mkdir -p /data/mail/
chown dovecot:dovecot /data/mail/

# Generate the dovecot ssl configuration based on all the certificates we have.
# The default goes first because dovecot complains otherwise.
echo "# Autogenerated by entrypoint.sh" > /etc/dovecot/auto-ssl.conf
cat >> /etc/dovecot/auto-ssl.conf <<EOF
ssl_cert = </etc/letsencrypt/live/$ONE_DOMAIN/fullchain.pem
ssl_key = </etc/letsencrypt/live/$ONE_DOMAIN/privkey.pem
EOF
for DOMAIN in $CERT_DOMAINS; do
	echo "local_name $DOMAIN {"
        echo "  ssl_cert = </etc/letsencrypt/live/$DOMAIN/fullchain.pem"
        echo "  ssl_key = </etc/letsencrypt/live/$DOMAIN/privkey.pem"
	echo "}"
done >> /etc/dovecot/auto-ssl.conf

# Pick the default domain as default hostname for chasquid. This is only used
# in plain text sessions and on very rare cases, and it's mostly for aesthetic
# purposes.
# Since the list of domains could have changed since the last run, always
# remove and re-add the setting for consistency.
sed -i '/^hostname:/d' /etc/chasquid/chasquid.conf
echo "hostname: '$ONE_DOMAIN'" >> /etc/chasquid/chasquid.conf

# Start the services: dovecot in background, chasquid in foreground.
start-stop-daemon --start --quiet --pidfile /run/dovecot.pid \
	--exec /usr/sbin/dovecot -- -c /etc/dovecot/dovecot.conf

sudo -u chasquid -g chasquid  /usr/bin/chasquid  $CHASQUID_FLAGS
