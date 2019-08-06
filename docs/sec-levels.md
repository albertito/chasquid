
# Security level checks

chasquid tracks per-domain TLS support, and uses it to prevent connection
downgrading.

Incoming and outgoing connections are tracked independently, but the principle
of operation is the same: once a domain shows it can establish secure
connections, chasquid will reject lower-security connections from/to its
servers.

This is very different from other MTAs, and has some tradeoffs.


## Outgoing connections

An outgoing connection has one of 3 security levels, which are (in order):

1. Plain: connection is plain-text (the server does not support TLS).
2. TLS insecure: TLS connection established, but the certificate itself was
   not valid.
3. TLS secure: TLS connection established, with a valid certificate.

When establishing an outgoing connection, chasquid will always attempt to
negotiate up to the *TLS secure* level.  After the negotiation, it will
compare which level it got, with the previously recorded value for this
domain:

* If the connection level is lower than the recorded value, then the
  connection will be dropped, and the delivery will fail (with a transient
  failure). The delivery will be retried as usual (using other MXs if
  available, and repeat after some delay).
* If the connection level is the same as the recorded value, then the
  connection will proceed.
* If the connection level is higher, chasquid will record this new value, and
  proceed.

If there is no previously recorded value for this domain, a *plain* level is
assumed.

### Certificate validation

A certificate is considered valid if it satisfies all of the following
conditions:

1. The certificate is properly signed by one of the system roots.
2. The name used to contact the server (e.g. the name from the MX record) is
   declared in the certificate.

This is the standard method used in other services such as HTTPS; however,
there is no standard to do certificate validation on SMTP.

chasquid chooses to implement validation this way, which is also consistent
with MTA-STS and HTTPS, but it is not universally agreed upon. It's also why
the "TLS insecure" state exists, instead of the connection being rejected
directly.


### Tradeoffs

Almost all other MTAs do TLS negotiation but accept *all* certificates, even
self-signed or expired ones.  chasquid operates differently, as described
above.

The main advantage is that, *with domains where secure connections were
previously established*, chasquid will detect connection downgrading (caused
by malicious interception such as STARTTLS blocking, as well as
misconfiguration such as incorrectly configured or expired certificates), and
avoid communicating insecurely.

The main disadvantage is that if a domain changes the configuration to a lower
security level, chasquid will fail the delivery (returning a message to the
sender explaining why).  Because there is no formal standard for TLS
certificate validation, and most MTAs will deliver email in this situation,
the domain owners might not see this as a problem and thus require [manual
intervention](#manual-override) on the chasquid side to explicitly allow it.

### MTA-STS

[MTA-STS](https://tools.ietf.org/html/rfc8461) is a relatively new standard
which defines a mechanism enabling mail service providers to declare their
ability to receive TLS connections, amongst other things.

It is supported by chasquid, out of the box, and in practice it means that for
domains that advertise MTA-STS support, the *secure* level will be enforced
even if the domain was previously unknown.


## Incoming connections

Incoming connections from authenticated users are always done over TLS
(chasquid will never accept authentication over plaintext connections). This
section applies only to incoming connections from other SMTP servers.

An incoming connection from another SMTP server is first checked through
[SPF](https://en.wikipedia.org/wiki/Sender_Policy_Framework). If the result of
the check is negative (fail, softfail, neutral, or error), then the following
is skipped. This prevents a malicious agent from raising the level and
interfering with legitimate plaintext delivery.

After the SPF check has passed, the connection is assigned one of the 2
security levels, which are (in order):

1. Plain: connection is plain-text (client did not do TLS negotiation).
2. TLS client: connection is over TLS.

At this point, chasquid will compare the level with the previously recorded
value for this domain:

* If the connection level is lower than the recorded value, then the
  connection is rejected with an SMTP error.
* If the connection level is the same as the recorded value, then the
  connection is allowed.
* If the connection level is higher, chasquid will record this new value, and
  the connection is allowed.

If there is no previously recorded value for this domain, a *plain* level is
assumed.

### Tradeoffs

Almost all other MTAs accept server to server connections regardless of the
security level, because there is no way for a domain to advertise that it will
always negotiate TLS when sending email. chasquid operates differently,
assuming that once a server negotiates TLS, it will always attempt to do so.

The main advantage is that, *with domains that had previously used TLS for
incoming connections*, chasquid will detect connection downgrading (caused by
malicious interception such as STARTTLS blocking), and avoid communicating
insecurely.

The main disadvantage is that if a domain changes the configuration and is
unable to negotiate TLS, chasquid will reject the connection and not receive
incoming email from this server. This is unusual nowadays, but because other
MTAs will accept the connection anyway, domain owners might not even notice
there is a problem, and might require [manual intervention](#manual-override)
on the chasquid side to explicitly allow it.



## Accepting lower security levels {#manual-override}

If a domain changes its configuration to a lower security level and is causing
chasquid to fail delivery, you can use
`chasquid-util domaininfo-remove <domain>` to make the server forget about
that domain.

Then, the next time there is a connection, there is no high security
expectation so it will proceed just fine, regardless of the level that was
negotiated.
