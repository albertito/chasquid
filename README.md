
# chasquid

[chasquid](https://blitiri.com.ar/p/chasquid) is an SMTP (email) server.

It aims to be easy to configure and maintain for a small mail server, at the
expense of flexibility and functionality.

It's written in [Go](https://golang.org), and distributed under the
[Apache license 2.0](http://en.wikipedia.org/wiki/Apache_License).

[![Build Status](https://travis-ci.org/albertito/chasquid.svg?branch=master)](https://travis-ci.org/albertito/chasquid)
[![Go Report Card](https://goreportcard.com/badge/github.com/albertito/chasquid)](https://goreportcard.com/report/github.com/albertito/chasquid)
[![Coverage Status](https://coveralls.io/repos/github/albertito/chasquid/badge.svg?branch=next)](https://coveralls.io/github/albertito/chasquid?branch=next)
[![GoDoc](https://godoc.org/blitiri.com.ar/go/chasquid?status.svg)](https://godoc.org/blitiri.com.ar/go/chasquid)


## Features

* Easy to configure.
* Hard to mis-configure in ways that are harmful or insecure (e.g. no open
  relay, or clear-text authentication).
* Tracking of per-domain TLS support, prevents connection downgrading.
* International usernames ([SMTPUTF8]) and domain names ([IDNA]).
* Hooks for easy integration with greylisting, anti-virus and anti-spam.
* Multiple domains, with per-domain user database and aliases.
* Multiple TLS certificates.
* Suffix dropping (`user+something@domain` → `user@domain`).
* Easy integration with [Let's Encrypt].
* [SPF] checking.
* Monitoring HTTP server, with exported variables and tracing to help
  debugging.
* Supports using [Dovecot] for authentication (experimental).

The following are intentionally *not* implemented:

* Custom email routing.
* [DKIM]/[DMARC] checking (although the post-data hook can be used for it).

[SMTPUTF8]: https://en.wikipedia.org/wiki/Extended_SMTP#SMTPUTF8
[IDNA]: https://en.wikipedia.org/wiki/Internationalized_domain_name
[Let's Encrypt]: https://letsencrypt.org
[Dovecot]: https://dovecot.org
[SPF]: https://en.wikipedia.org/wiki/Sender_Policy_Framework
[DKIM]: https://en.wikipedia.org/wiki/DomainKeys_Identified_Mail
[DMARC]: https://en.wikipedia.org/wiki/DMARC


## Status

chasquid is in beta.

It's functional and has had some production exposure, but some things may
still change in backwards-incompatible way, including the configuration format.
It should be rare and will be avoided if possible.

You can subscribe to the mailing list to get notifications of such changes,
which are also documented in the [UPGRADING](UPGRADING.md) file.


## Documentation

Check out the [how-to](docs/howto.md) or the [installation guide](INSTALL.md)
for more details on how to install and configure chasquid.


## Contact

If you have any questions, comments or patches please send them to the mailing
list, chasquid@googlegroups.com.

To subscribe, send an email to chasquid+subscribe@googlegroups.com.

You can also browse the
[archives](https://groups.google.com/forum/#!forum/chasquid).

