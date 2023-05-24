
# chasquid

[chasquid](https://blitiri.com.ar/p/chasquid) is an SMTP (email) server with a
focus on simplicity, security, and ease of operation.

It is designed mainly for individuals and small groups.

It's written in [Go](https://golang.org), and distributed under the
[Apache license 2.0](http://en.wikipedia.org/wiki/Apache_License).

[![Go tests](https://github.com/albertito/chasquid/actions/workflows/gotests.yml/badge.svg?branch=master)](https://github.com/albertito/chasquid/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/albertito/chasquid)](https://goreportcard.com/report/github.com/albertito/chasquid)
[![Coverage](https://img.shields.io/badge/coverage-next-brightgreen.svg)](https://blitiri.com.ar/p/chasquid/coverage.html)  
[![Docs](https://img.shields.io/badge/docs-reference-blue.svg)](https://blitiri.com.ar/p/chasquid/)
[![OFTC IRC](https://img.shields.io/badge/chat-oftc-blue.svg)](https://webchat.oftc.net/?channels=%23chasquid)


## Features

* Easy
    * Easy to configure.
    * Hard to mis-configure in ways that are harmful or insecure (e.g. no open
      relay, or clear-text authentication).
    * [Monitoring] HTTP server, with exported variables and tracing to help
      debugging.
    * Integrated with [Debian], [Ubuntu], and [Arch].
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


[Arch]: https://blitiri.com.ar/p/chasquid/install/#arch
[Debian]: https://blitiri.com.ar/p/chasquid/install/#debianubuntu
[Dovecot]: https://blitiri.com.ar/p/chasquid/dovecot/
[Hooks]: https://blitiri.com.ar/p/chasquid/hooks/
[IDNA]: https://en.wikipedia.org/wiki/Internationalized_domain_name
[Let's Encrypt]: https://letsencrypt.org
[MTA-STS]: https://tools.ietf.org/html/rfc8461
[Monitoring]: https://blitiri.com.ar/p/chasquid/monitoring/
[SMTPUTF8]: https://en.wikipedia.org/wiki/Extended_SMTP#SMTPUTF8
[SPF]: https://en.wikipedia.org/wiki/Sender_Policy_Framework
[Tracking]: https://blitiri.com.ar/p/chasquid/sec-levels/
[Ubuntu]: https://blitiri.com.ar/p/chasquid/install/#debianubuntu


## Documentation

The [how-to guide](https://blitiri.com.ar/p/chasquid/howto/) and the
[installation guide](https://blitiri.com.ar/p/chasquid/install/) are the
best starting points on how to install, configure and run chasquid.

You will find [all documentation here](https://blitiri.com.ar/p/chasquid/).


## Contact

If you have any questions, comments or patches, please send them to the [mailing
list](https://groups.google.com/forum/#!forum/chasquid),
chasquid@googlegroups.com.  
To subscribe, send an email to chasquid+subscribe@googlegroups.com.

Security issues can be reported privately to albertito@blitiri.com.ar.

Bug reports and pull requests on
[GitHub](https://github.com/albertito/chasquid) are also welcome.

You can also reach out via IRC, `#chasquid` on [OFTC](https://oftc.net/).

