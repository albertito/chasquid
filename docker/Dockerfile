# Docker file for creating a container that will run chasquid and Dovecot.
#
# THIS IS EXPERIMENTAL AND LIKELY TO CHANGE.
#
# This is not recommended for serious installations, you're probably better
# off following the documentation and setting the server up manually.
#
# See the README.md file for more details.

# Build the binaries.
FROM golang:latest AS build
WORKDIR /go/src/blitiri.com.ar/go/chasquid
COPY . .
RUN go get -d ./...
RUN go install ./...

# Create the image.
FROM debian:stable

# Make debconf/frontend non-interactive, to avoid distracting output about the
# lack of $TERM.
ENV DEBIAN_FRONTEND=noninteractive

# Install the packages we need.
# This includes chasquid, which sets up good defaults.
RUN apt-get update -q
RUN apt-get install -y -q \
	chasquid \
	dovecot-lmtpd dovecot-imapd dovecot-pop3d \
	dovecot-sieve dovecot-managesieved \
	acl libcap2-bin sudo certbot

# Copy the binaries. This overrides the debian packages with the ones we just
# built above.
COPY --from=build /go/bin/chasquid /usr/bin/
COPY --from=build /go/bin/chasquid-util /usr/bin/
COPY --from=build /go/bin/smtp-check /usr/bin/
COPY --from=build /go/bin/mda-lmtp /usr/bin/

# Let chasquid bind privileged ports, so we can run it as its own user.
RUN setcap CAP_NET_BIND_SERVICE=+eip /usr/bin/chasquid

# Copy docker-specific configurations.
COPY docker/dovecot.conf /etc/dovecot/dovecot.conf
COPY docker/chasquid.conf /etc/chasquid/chasquid.conf

# Copy utility scripts.
COPY docker/add-user.sh /
COPY docker/entrypoint.sh /

# chasquid: SMTP, submission, submission+tls.
EXPOSE 25 465 587

# dovecot: POP3s, IMAPs, managesieve.
EXPOSE 993 995 4190

# http for letsencrypt/certbot.
EXPOSE 80 443

# Store emails and chasquid databases in an external volume, to be mounted at
# /data, so they're independent from the image itself.
VOLUME /data

# Put some directories where we expect persistent user data into /data.
RUN rmdir /etc/chasquid/domains/
RUN ln -sf /data/chasquid/domains/ /etc/chasquid/domains
RUN rm -rf /etc/letsencrypt/
RUN ln -sf /data/letsencrypt/ /etc/letsencrypt

# Give the chasquid user access to the necessary configuration.
RUN setfacl -R -m u:chasquid:rX /etc/chasquid/
RUN mv /etc/chasquid/certs/ /etc/chasquid/certs-orig
RUN ln -s /etc/letsencrypt/live/ /etc/chasquid/certs


# NOTE: Set AUTO_CERTS="example.com example.org" to automatically obtain and
# renew certificates upon startup, via Letsencrypt. You're agreeing to their
# ToS by setting this variable, so please review them carefully.
# CERTS_EMAIL should be set to your email address so letsencrypt can send you
# critical notifications.

# Custom entry point that does some configuration checks and ensures
# letsencrypt is properly set up.
ENTRYPOINT ["/entrypoint.sh"]
