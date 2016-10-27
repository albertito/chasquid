
ifndef VERSION
    VERSION = `git describe --always --long --dirty`
endif

# https://wiki.debian.org/ReproducibleBuilds/TimestampsProposal
ifndef SOURCE_DATE_EPOCH
    SOURCE_DATE_EPOCH = `git log -1 --format=%ct`
endif


default: chasquid

all: chasquid chasquid-util smtp-check spf-check


chasquid:
	go build -ldflags="\
		-X main.version=${VERSION} \
		-X main.sourceDateTs=${SOURCE_DATE_EPOCH} \
		" ${GOFLAGS}


chasquid-util:
	go build ${GOFLAGS} ./cmd/chasquid-util/

smtp-check:
	go build ${GOFLAGS} ./cmd/smtp-check/

spf-check:
	go build ${GOFLAGS} ./cmd/spf-check/


test:
	go test ${GOFLAGS} ./...
	setsid -w ./test/run.sh
	setsid -w ./cmd/chasquid-util/test.sh


install-binaries: chasquid chasquid-util smtp-check
	mkdir -p /usr/local/bin/
	cp -a chasquid chasquid-util smtp-check /usr/local/bin/

install-config-skeleton:
	if ! [ -d /etc/chasquid ] ; then cp -arv etc / ; fi
	
	if ! [ -d /var/lib/chasquid ]; then \
		mkdir -v /var/lib/chasquid; \
		chmod -v 0700 /var/lib/chasquid ; \
		chown -v mail:mail /var/lib/chasquid ; \
	fi


.PHONY: chasquid chasquid-util smtp-check spf-check test
