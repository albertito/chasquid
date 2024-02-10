
# DKIM integration

[chasquid] supports verifying and generating [DKIM] signatures since version
1.14.

All incoming email is verified, and *authenticated* emails for domains which
have a private DKIM key set up will be signed.

In versions older than 1.13, support is possible via the [hooks] mechanism. In
particular, the [example hook] included support for some command-line
implementations. That continues to be an option, especially if customization
is needed.


## Easy setup

- Run `chasquid-util dkim-keygen DOMAIN` to generate a DKIM private key for
  your domain. The file will be in `/etc/chasquid/domains/DOMAIN/dkim:*.pem`.
- Publish the DKIM DNS record which was shown by the
  previous command (e.g. by following
  [this guide](https://support.dnsimple.com/articles/dkim-record/)).
- Change the key file's permissions, to ensure it is readable by chasquid (and
  nobody else).
- Restart chasquid.

It is highly recommended that you use a DKIM checker (like
[Learn DMARC](https://www.learndmarc.com/)) to confirm that your setup is
fully functional.


## Advanced setup

You need to place the PEM-encoded private key in the domain config directory,
with a name like `dkim:SELECTOR.pem`, where `SELECTOR` is the selector string.

It needs to be either RSA or Ed25519.

### Key rotation

To rotate a key, you can remove the old key file, and generate a new one as
per the previous step.

It is important to remove the old key from the directory, because chasquid
will use *all* the keys in it.

You should use a different selector each time. If you don't specify a
selector when using `chasquid-util dkim-keygen`, the current date will be
used, which is a safe default to prevent accidental reuse.


### Multiple keys

Advanced users may want to sign outgoing mail with multiple keys (e.g. to
support multiple signing algorithms).

This is well supported: chasquid will sign email with all keys it find that
match `dkim:*.pem` in a domain directory.


## Verification

[chasquid] will verify all DKIM signatures of incoming mail, and record the
results in an [`Authentication-Results:`] header, as per [RFC 8601].

Note that emails will *not* be rejected even if they fail verification, as
this is not recommended
([source 1](https://tools.ietf.org/html/rfc6376#section-6.3),
[source 2](https://tools.ietf.org/html/rfc7601#section-2.7.1)).


## Other implementations

[chasquid] also supports [DKIM] via the [hooks] mechanism. This can be useful
if more customization is needed.

Implementations that have been tried:

- [driusan/dkim]
- [dkimpy]


[chasquid]: https://blitiri.com.ar/p/chasquid
[DKIM]: https://en.wikipedia.org/wiki/DomainKeys_Identified_Mail
[hooks]: hooks.md
[example hook]: https://blitiri.com.ar/git/r/chasquid/b/next/t/etc/chasquid/hooks/f=post-data.html
[driusan/dkim]: https://github.com/driusan/dkim
[dkimpy]: https://launchpad.net/dkimpy/
[RFC 8601]: https://datatracker.ietf.org/doc/html/rfc8601
[`Authentication-Results:`]: https://en.wikipedia.org/wiki/Email_authentication#Authentication-Results
