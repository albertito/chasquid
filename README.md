
# chasquid

[chasquid](https://blitiri.com.ar/p/chasquid) is an SMTP (email) server with a
focus on simplicity, security, and ease of operation.

It's written in [Go](https://golang.org), and distributed under the
[Apache license 2.0](http://en.wikipedia.org/wiki/Apache_License).

[![Travis-CI build status](https://travis-ci.org/albertito/chasquid.svg?branch=master)](https://travis-ci.org/albertito/chasquid)
[![Go Report Card](https://goreportcard.com/badge/github.com/albertito/chasquid)](https://goreportcard.com/report/github.com/albertito/chasquid)
[![Coverage Status](https://coveralls.io/repos/github/albertito/chasquid/badge.svg?branch=next)](https://coveralls.io/github/albertito/chasquid?branch=next)
[![GoDoc](https://godoc.org/blitiri.com.ar/go/chasquid?status.svg)](https://godoc.org/blitiri.com.ar/go/chasquid)
[![Freenode](https://img.shields.io/badge/chat-freenode-brightgreen.svg)](https://webchat.freenode.net?channels=%23chasquid)


## Features

* Easy
    * Easy to configure.
    * Hard to mis-configure in ways that are harmful or insecure (e.g. no open
      relay, or clear-text authentication).
    * Monitoring HTTP server, with exported variables and tracing to help
      debugging.
    * Integrated with [Debian] and [Ubuntu].
    * Supports using [Dovecot] for authentication.
* Useful
    * Multiple/virtual domains, with per-domain users and aliases.
    * Suffix dropping (`user+something@domain` → `user@domain`).
    * Hooks for integration with greylisting, anti-virus, anti-spam, and
      DKIM/DMARC.
    * International usernames ([SMTPUTF8]) and domain names ([IDNA]).
* Secure
    * Tracking of per-domain TLS support, prevents connection downgrading.
    * Multiple TLS certificates.
    * Easy integration with [Let's Encrypt].
    * [SPF] and [MTA-STS] checking.


[SMTPUTF8]: https://en.wikipedia.org/wiki/Extended_SMTP#SMTPUTF8
[IDNA]: https://en.wikipedia.org/wiki/Internationalized_domain_name
[Let's Encrypt]: https://letsencrypt.org
[Dovecot]: https://dovecot.org
[SPF]: https://en.wikipedia.org/wiki/Sender_Policy_Framework
[MTA-STS]: https://tools.ietf.org/html/rfc8461
[Debian]: https://debian.org
[Ubuntu]: https://ubuntu.com


## Status

chasquid is in beta.

It's functional and has had some production exposure, but some things may
still change in backwards-incompatible way, including the configuration format.
It should be rare and will be avoided if possible.

You can subscribe to the mailing list to get notifications of such changes,
which are also documented in the [UPGRADING](UPGRADING.md) file.


## Documentation

The [Debian+Dovecot+chasquid how-to](docs/howto.md) and the
[installation guide](INSTALL.md)
are the best starting points on how to install, configure and run chasquid.


## Contact

If you have any questions, comments or patches please send them to the [mailing
list](https://groups.google.com/forum/#!forum/chasquid),
chasquid@googlegroups.com.

To subscribe, send an email to chasquid+subscribe@googlegroups.com.

You can also reach out via IRC, `#chasquid` on
[freenode](https://freenode.net/).


