
ifndef VERSION
    VERSION = `git describe --always --long --dirty --tags`
endif

# https://wiki.debian.org/ReproducibleBuilds/TimestampsProposal
ifndef SOURCE_DATE_EPOCH
    SOURCE_DATE_EPOCH = `git log -1 --format=%ct`
endif


default: chasquid

all: chasquid chasquid-util smtp-check mda-lmtp dovecot-auth-cli


chasquid:
	go build -ldflags="\
		-X main.version=${VERSION} \
		-X main.sourceDateTs=${SOURCE_DATE_EPOCH} \
		" ${GOFLAGS}


chasquid-util:
	go build ${GOFLAGS} ./cmd/chasquid-util/

smtp-check:
	go build ${GOFLAGS} ./cmd/smtp-check/

mda-lmtp:
	go build ${GOFLAGS} ./cmd/mda-lmtp/

dovecot-auth-cli:
	go build ${GOFLAGS} ./cmd/dovecot-auth-cli/

test:
	go test ${GOFLAGS} ./...
	setsid -w ./test/run.sh
	setsid -w ./test/stress.sh
	setsid -w ./cmd/chasquid-util/test.sh
	setsid -w ./cmd/mda-lmtp/test.sh
	setsid -w ./cmd/dovecot-auth-cli/test.sh


install-binaries: chasquid chasquid-util smtp-check mda-lmtp
	mkdir -p /usr/local/bin/
	cp -a chasquid chasquid-util smtp-check mda-lmtp /usr/local/bin/

install-config-skeleton:
	if ! [ -d /etc/chasquid ] ; then cp -arv etc / ; fi
	
	if ! [ -d /var/lib/chasquid ]; then \
		mkdir -v /var/lib/chasquid; \
		chmod -v 0700 /var/lib/chasquid ; \
		chown -v mail:mail /var/lib/chasquid ; \
	fi

fmt:
	go vet ./...
	gofmt -s -w .
	clang-format -i $(shell find . -iname '*.proto')

.PHONY: chasquid test \
	chasquid-util smtp-check mda-lmtp dovecot-auth-cli \
	fmt
