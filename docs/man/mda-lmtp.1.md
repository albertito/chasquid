# NAME

mda-lmtp - MDA that uses LMTP to do the mail delivery

# SYNOPSIS

mda-lmtp
\[**-addr\_network** _net_\]
**-addr** _addr_
**-d** _recipient_
**-f** _from_

# DESCRIPTION

mda-lmtp is a very basic MDA that uses LMTP to do the mail delivery.

It takes command line arguments similar to maildrop or procmail, reads an
email via standard input, and sends it over the given LMTP server.  Supports
connecting to LMTP servers over UNIX sockets and TCP.

It can be used when your mail server does not support LMTP directly.

# EXAMPLE

**mda-lmtp** _--addr=localhost:1234_ _-f juan@casa_ _-d jose_ < email

# OPTIONS

- **-addr** _address_

    LMTP server address to connect to.

- **-addr\_network** _network_

    Network of the LMTP address (e.g. _unix_ or _tcp_). If not present, it will
    be autodetected from the address itself.

- **-d** _recipient_

    Recipient.

- **-f** _from_

    Whom the message is from.

# RETURN VALUES

- **0**

    success

- **75**

    temporary failure

- _other_

    permanent failures (usually indicate misconfiguration)

# SEE ALSO

[chasquid(1)](chasquid.1.md)
