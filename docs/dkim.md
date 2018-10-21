
# DKIM integration

[chasquid] supports generating [DKIM] signatures via the [hooks](hooks.md)
mechanism.


## Signing

The example hook in this repository contains an example of integration with
[driusan/dkim](https://github.com/driusan/dkim) tools, and assumes the
following:

- The [selector](https://tools.ietf.org/html/rfc6376#section-3.1) for a domain
  can be found in the file `domains/$DOMAIN/dkim_selector`.
- The private key to use for signing can be found in the file
  `certs/$DOMAIN/dkim_privkey.pem`.

Only authenticated email will be signed.


## Verification

Verifying signatures is technically supported as well, and can be done in the
same hook. However, it's not recommended for SMTP servers to reject mail on
verification failures
([source 1](https://tools.ietf.org/html/rfc6376#section-6.3),
[source 2](https://tools.ietf.org/html/rfc7601#section-2.7.1)), so it is not
included in the example.


[chasquid]: https://blitiri.com.ar/p/chasquid
[DKIM]: https://en.wikipedia.org/wiki/DomainKeys_Identified_Mail
