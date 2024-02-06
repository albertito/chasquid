
# Clients

chasquid supports most SMTP clients, but requires them to have some features:

- Support TLS (either
  [STARTTLS](https://datatracker.ietf.org/doc/html/rfc3207) or
  [implicit TLS](https://datatracker.ietf.org/doc/html/rfc8314#section-3.3))
- Support the
  [PLAIN authentication method](https://datatracker.ietf.org/doc/html/rfc4954#section-4).

All modern clients should support both, and thus have no problems talking to
chasquid.


## Configuration examples

### [msmtp](https://marlam.de/msmtp/)

This example is useful as either per-user `~/.msmtprc` or system-wide
`/etc/msmtprc`:

```
account default
tls on
auth on

# Use the SMTP submission port. Many providers block communications to the
# default port 25, but the submission port 587 tends to work just fine.
port 587

# Server hostname.
host SERVER

# Your username (including the domain).
user USER@DOMAIN

# Your password.
password SECRET
```

Replace the `SERVER`, `USER@DOMAIN` and `SECRET` strings with the appropriate
values.


## Problematic clients

These clients are known to have issues talking to chasquid:

- [ssmtp](https://packages.debian.org/source/unstable/ssmtp): does not
  support the PLAIN authentication method. It is also unmaintained.  
  Please use [msmtp](https://marlam.de/msmtp/) instead.


