# chasquid

[chasquid](https://blitiri.com.ar/p/chasquid) is an SMTP (email) server with a
focus on simplicity, security, and ease of operation.

It is designed mainly for individuals and small groups.

It's written in [Go](https://golang.org), and distributed under the
[Apache license 2.0](http://en.wikipedia.org/wiki/Apache_License).

[![Go tests](https://github.com/albertito/chasquid/actions/workflows/gotests.yml/badge.svg?branch=main)](https://github.com/albertito/chasquid/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/albertito/chasquid)](https://goreportcard.com/report/github.com/albertito/chasquid)
[![Coverage](https://coveralls.io/repos/github/albertito/chasquid/badge.svg?branch=next)](https://coveralls.io/github/albertito/chasquid?branch=next)\
[![Docs](https://img.shields.io/badge/docs-reference-blue.svg)](https://blitiri.com.ar/p/chasquid/)
[![OFTC IRC](https://img.shields.io/badge/chat-oftc-blue.svg)](https://webchat.oftc.net/?channels=%23chasquid)

## Features

- Easy
  - Easy to configure.
  - Hard to mis-configure in ways that are harmful or insecure (e.g. no open
    relay, or clear-text authentication).
  - [Monitoring] HTTP server, with exported variables and tracing to help
    debugging.
  - Integrated with [Debian], [Ubuntu], [Arch], and [Alpine].
  - Supports using [Dovecot] for authentication.
- Useful
  - Multiple/virtual domains, with per-domain users and aliases.
  - Suffix dropping (`user+something@domain` → `user@domain`).
  - [Hooks] for integration with greylisting, anti-virus, and anti-spam.
  - International usernames ([SMTPUTF8]) and domain names ([IDNA]).
- Secure
  - [Tracking] of per-domain TLS support, prevents connection downgrading.
  - Multiple TLS certificates.
  - Easy integration with [Let's Encrypt].
  - [SPF] and [MTA-STS] checking.
  - [DKIM] support (signing and verification).

## Documentation

The [how-to guide](https://blitiri.com.ar/p/chasquid/howto/) and the
[installation guide](https://blitiri.com.ar/p/chasquid/install/) are the best
starting points on how to install, configure and run chasquid.

You will find [all documentation here](https://blitiri.com.ar/p/chasquid/).

## Contact

If you have any questions, comments or patches, please send them to the
[mailing list](https://groups.google.com/forum/#!forum/chasquid),
chasquid@googlegroups.com.\
To subscribe, send an email to chasquid+subscribe@googlegroups.com.

Security issues can be reported privately to albertito@blitiri.com.ar.

Bug reports and pull requests on
[GitHub](https://github.com/albertito/chasquid) are also welcome.

You can also reach out via IRC, `#chasquid` on [OFTC](https://oftc.net/).

[alpine]: https://blitiri.com.ar/p/chasquid/install/#alpine
[arch]: https://blitiri.com.ar/p/chasquid/install/#arch
[debian]: https://blitiri.com.ar/p/chasquid/install/#debianubuntu
[dkim]: https://en.wikipedia.org/wiki/DomainKeys_Identified_Mail
[dovecot]: https://blitiri.com.ar/p/chasquid/dovecot/
[hooks]: https://blitiri.com.ar/p/chasquid/hooks/
[idna]: https://en.wikipedia.org/wiki/Internationalized_domain_name
[let's encrypt]: https://letsencrypt.org
[monitoring]: https://blitiri.com.ar/p/chasquid/monitoring/
[mta-sts]: https://tools.ietf.org/html/rfc8461
[smtputf8]: https://en.wikipedia.org/wiki/Extended_SMTP#SMTPUTF8
[spf]: https://en.wikipedia.org/wiki/Sender_Policy_Framework
[tracking]: https://blitiri.com.ar/p/chasquid/sec-levels/
[ubuntu]: https://blitiri.com.ar/p/chasquid/install/#debianubuntu
