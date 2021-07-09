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
	exim4-daemon-light \
	haproxy \
	python3-dkim

# Install sudo, needed for the docker entrypoint.
RUN apt-get install -y -q sudo

# Prepare exim.
RUN mkdir -p test/t-02-exim/.exim4 \
	&& ln -s /usr/sbin/exim4 test/t-02-exim/.exim4

# Prepare msmtp: remove setuid, otherwise HOSTALIASES doesn't work.
RUN chmod g-s /usr/bin/msmtp

# Install binaries for the (optional) DKIM integration test.
RUN go install github.com/driusan/dkim/cmd/dkimsign@latest \
	&& go install github.com/driusan/dkim/cmd/dkimverify@latest \
	&& go install github.com/driusan/dkim/cmd/dkimkeygen@latest

# Copy into the container. Everything below this line will not be cached.
COPY . .

# Don't run the tests as root: it makes some integration tests more difficult,
# as for example Exim has hard-coded protections against running as root.
RUN useradd -m chasquid && chown -R chasquid:chasquid .

# Update dependencies to the latest versions, and fetch them to the cache.
# The fetch is important because once within the entrypoint, we no longer have
# network access to the outside, so all modules need to be available.
# Do it as chasquid because that is what the tests will run as.
USER chasquid
ENV GOPATH=
RUN go get -v ${GO_GET_ARGS} ./... && go mod download

# Build the minidns server, which will be run from within the entrypoint.
RUN go build -o /tmp/minidns ./test/util/minidns.go
USER root

# Custom entry point, which uses our own DNS server.
ENTRYPOINT ["./test/util/docker_entrypoint.sh"]
