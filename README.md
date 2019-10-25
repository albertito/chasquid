
# chasquid

[chasquid](https://blitiri.com.ar/p/chasquid) is an SMTP (email) server with a
focus on simplicity, security, and ease of operation.

It is designed mainly for individuals and small groups.

It's written in [Go](https://golang.org), and distributed under the
[Apache license 2.0](http://en.wikipedia.org/wiki/Apache_License).

[![Travis-CI status](https://travis-ci.org/albertito/chasquid.svg?branch=master)](https://travis-ci.org/albertito/chasquid)
[![Gitlab CI status](https://gitlab.com/albertito/chasquid/badges/master/pipeline.svg)](https://gitlab.com/albertito/chasquid/pipelines)
[![Go Report Card](https://goreportcard.com/badge/github.com/albertito/chasquid)](https://goreportcard.com/report/github.com/albertito/chasquid)
[![Coverage](https://img.shields.io/badge/coverage-next-brightgreen.svg)](https://blitiri.com.ar/p/chasquid/coverage.html)
[![Docs](https://img.shields.io/badge/docs-reference-blue.svg)](https://blitiri.com.ar/p/chasquid/docs/)
[![Freenode](https://img.shields.io/badge/chat-freenode-blue.svg)](https://webchat.freenode.net/#chasquid)


## Features

* Easy
    * Easy to configure.
    * Hard to mis-configure in ways that are harmful or insecure (e.g. no open
      relay, or clear-text authentication).
    * [Monitoring] HTTP server, with exported variables and tracing to help
      debugging.
    * Integrated with [Debian] and [Ubuntu].
    * Supports using [Dovecot] for authentication.
* Useful
    * Multiple/virtual domains, with per-domain users and aliases.
    * Suffix dropping (`user+something@domain` → `user@domain`).
    * [Hooks] for integration with greylisting, anti-virus, anti-spam, and
      DKIM/DMARC.
    * International usernames ([SMTPUTF8]) and domain names ([IDNA]).
* Secure
    * [Tracking] of per-domain TLS support, prevents connection downgrading.
    * Multiple TLS certificates.
    * Easy integration with [Let's Encrypt].
    * [SPF] and [MTA-STS] checking.


[Debian]: https://debian.org
[Dovecot]: https://blitiri.com.ar/p/chasquid/docs/dovecot/
[Hooks]: https://blitiri.com.ar/p/chasquid/docs/hooks/
[IDNA]: https://en.wikipedia.org/wiki/Internationalized_domain_name
[Let's Encrypt]: https://letsencrypt.org
[MTA-STS]: https://tools.ietf.org/html/rfc8461
[Monitoring]: https://blitiri.com.ar/p/chasquid/docs/monitoring/
[SMTPUTF8]: https://en.wikipedia.org/wiki/Extended_SMTP#SMTPUTF8
[SPF]: https://en.wikipedia.org/wiki/Sender_Policy_Framework
[Tracking]: https://blitiri.com.ar/p/chasquid/docs/sec-levels/
[Ubuntu]: https://ubuntu.com


## Documentation

The [how-to guide](https://blitiri.com.ar/p/chasquid/docs/howto/) and the
[installation guide](https://blitiri.com.ar/p/chasquid/docs/install/) are the
best starting points on how to install, configure and run chasquid.

You will find [all documentation here](https://blitiri.com.ar/p/chasquid/docs/).


## Contact

If you have any questions, comments or patches please send them to the [mailing
list](https://groups.google.com/forum/#!forum/chasquid),
chasquid@googlegroups.com.

To subscribe, send an email to chasquid+subscribe@googlegroups.com.

You can also reach out via IRC, `#chasquid` on
[freenode](https://freenode.net/).


