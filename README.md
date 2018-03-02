
# chasquid

[chasquid](https://blitiri.com.ar/p/chasquid) is an SMTP (email) server.

It aims to be easy to configure and maintain for a small mail server, at the
expense of flexibility and functionality.

It's written in [Go](https://golang.org).


## Features

* Easy to configure, hard to mis-configure in ways that are harmful or
  insecure (e.g. no open relay, clear-text authentication, etc.).
* Tracking of per-domain TLS support, prevents connection downgrading.
* SMTP UTF8 (international usernames).
* IDNA (international domain names).
* Hooks for easy integration with greylisting, anti-virus and anti-spam.
* Multiple domains, with per-domain user database and aliases.
* Multiple TLS certificates.
* Suffix dropping (user+something@domain -> user@domain).
* Easy integration with letsencrypt.
* SPF checking.
* Monitoring HTTP server, with exported variables and tracing to help
  debugging.
* Using dovecot for authentication (experimental).

The following are intentionally *not* implemented:

* Custom email routing and transport.
* DKIM/DMARC checking (although the post-data hook can be used for it).


## Status

chasquid is in beta.

It's functional and has had some production exposure, but some things may
still change in backwards-incompatible way, including the configuration format.
It should be rare and will be avoided if possible.

You should subscribe to the mailing list to get notifications of such changes.


## Contact

If you have any questions, comments or patches please send them to the mailing
list, chasquid@googlegroups.com.

To subscribe, send an email to chasquid+subscribe@googlegroups.com.

You can also browse the
[archives](https://groups.google.com/forum/#!forum/chasquid).


## Documentation

Check out the [how-to](docs/howto.md) or the [installation guide](INSTALL.md)
for more details on how to install and configure chasquid.

