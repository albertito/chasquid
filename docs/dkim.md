
# DKIM integration

[chasquid] supports generating [DKIM] signatures via the [hooks](hooks.md)
mechanism.


## Signing

The [example hook] includes integration with [driusan/dkim] and [dkimpy], and
assumes the following:

- The [selector](https://tools.ietf.org/html/rfc6376#section-3.1) for a domain
  can be found in the file `domains/$DOMAIN/dkim_selector`.
- The private key to use for signing can be found in the file
  `certs/$DOMAIN/dkim_privkey.pem`.

Only authenticated email will be signed.


### Setup with [driusan/dkim]

1. Install the [driusan/dkim] tools with something like the following (adjust
   to your local environment):

     ```
     for i in dkimsign dkimverify dkimkeygen; do
     	go get github.com/driusan/dkim/cmd/$i
     	go install github.com/driusan/dkim/cmd/$i
     done
     sudo cp ~/go/bin/{dkimsign,dkimverify,dkimkeygen} /usr/local/bin
     ```

1. Generate the domain key for your domain using `dkimkeygen`.
1. Publish the DNS record from `dns.txt`
   ([guide](https://support.dnsimple.com/articles/dkim-record/)).
1. Write the selector you chose to `domains/$DOMAIN/dkim_selector`.
1. Copy `private.pem` to `/etc/chasquid/certs/$DOMAIN/dkim_privkey.pem`.
1. Verify the setup using one of the publicly available tools, like
   [mail-tester](https://www.mail-tester.com/spf-dkim-check).


### Setup with [dkimpy]

1. Install [dkimpy] with `apt install python3-dkim` or the equivalent for your
   environment.
1. Generate the domain key for your domain using `dknewkey dkim`.
1. Publish the DNS record from `dkim.dns`
   ([guide](https://support.dnsimple.com/articles/dkim-record/)).
1. Write the selector you chose to `domains/$DOMAIN/dkim_selector`.
1. Copy `dkim.key` to `/etc/chasquid/certs/$DOMAIN/dkim_privkey.pem`.
1. Verify the setup using one of the publicly available tools, like
   [mail-tester](https://www.mail-tester.com/spf-dkim-check).


## Verification

Verifying signatures is technically supported as well, and can be done in the
same hook. However, it's not recommended for SMTP servers to reject mail on
verification failures
([source 1](https://tools.ietf.org/html/rfc6376#section-6.3),
[source 2](https://tools.ietf.org/html/rfc7601#section-2.7.1)), so it is not
included in the example.


[chasquid]: https://blitiri.com.ar/p/chasquid
[DKIM]: https://en.wikipedia.org/wiki/DomainKeys_Identified_Mail
[example hook]: https://blitiri.com.ar/git/r/chasquid/b/next/t/etc/chasquid/hooks/f=post-data.html
[driusan/dkim]: https://github.com/driusan/dkim
[dkimpy]: https://launchpad.net/dkimpy/
