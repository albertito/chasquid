# NAME

smtp-check - SMTP setup checker

# SYNOPSIS

**smtp-check** \[-port _port_\] \[-localname _domain_\] \[-skip\_tls\_check\] _domain_

# DESCRIPTION

smtp-check is a command-line too for checking SMTP setups (DNS records, TLS
certificates, SPF, etc.).

# OPTIONS

- **-port** _port_:

    Port to use for connecting to the MX servers.

- **-localname** _domain_:

    Local name to use for the EHLO command.

- **-skip\_tls\_check**:

    Skip TLS check (useful if connections are blocked).

# SEE ALSO

[chasquid(1)](chasquid.1.md)
