# Docker file for creating a docker container that can run the tests.
#
# Create the image:
#   docker build -t chasquid-test -f test/Dockerfile .
#
# Run the tests:
#   docker run --rm chasquid-test  make test
#
# Get a shell inside the image (for debugging):
#   docker run -it --entrypoint=/bin/bash chasquid-test

FROM golang:latest

WORKDIR /go/src/blitiri.com.ar/go/chasquid
COPY . .

# Make debconf/frontend non-interactive, to avoid distracting output about the
# lack of $TERM.
ENV DEBIAN_FRONTEND noninteractive

# Install the basics for the integration tests.
RUN apt-get update -q
RUN apt-get install -y -q python3 msmtp

# Packages for the (optional) dovecot integration test.
RUN apt-get install -y -q gettext-base dovecot-imapd

# Packages for the (optional) exim integration test.
RUN apt-get install -y -q exim4-daemon-light
RUN cd test/t-02-exim && mkdir -p .exim4 && ln -s /usr/sbin/exim4 .exim4/

RUN go get -d ./...
RUN go install ./...

# Don't run the tests as root: it makes some integration tests more difficult,
# as for example Exim has hard-coded protections against running as root.
RUN useradd -m chasquid
RUN chown -R chasquid:chasquid .
USER chasquid

# Tests expect the $USER variable set.
ENV USER chasquid
