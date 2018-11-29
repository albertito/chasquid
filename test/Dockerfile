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

# Make debconf/frontend non-interactive, to avoid distracting output about the
# lack of $TERM.
ENV DEBIAN_FRONTEND noninteractive

RUN apt-get update -q

# Install the required packages for the integration tests.
RUN apt-get install -y -q python3 msmtp

# Install the optional packages for the integration tests.
RUN apt-get install -y -q \
	gettext-base dovecot-imapd \
	exim4-daemon-light

# Install sudo, needed for the docker entrypoint.
RUN apt-get install -y -q sudo

# Prepare exim.
RUN mkdir -p test/t-02-exim/.exim4 \
	&& ln -s /usr/sbin/exim4 test/t-02-exim/.exim4

# Install binaries for the (optional) DKIM integration test.
RUN go get github.com/driusan/dkim/... \
	&& go install github.com/driusan/dkim/cmd/dkimsign \
	&& go install github.com/driusan/dkim/cmd/dkimverify \
	&& go install github.com/driusan/dkim/cmd/dkimkeygen

# Copy into the container. Everything below this line will not be cached.
COPY . .

# Install chasquid and its dependencies.
RUN go get -d -v ./... && go install -v ./...

# Custom entry point, which uses our own DNS server.
ENTRYPOINT ["./test/util/docker_entrypoint.sh"]

# Don't run the tests as root: it makes some integration tests more difficult,
# as for example Exim has hard-coded protections against running as root.
RUN useradd -m chasquid && chown -R chasquid:chasquid .
#USER chasquid

# Tests expect the $USER variable set.
#ENV USER chasquid
