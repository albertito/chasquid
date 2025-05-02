# NAME

[chasquid.conf(5)](chasquid.conf.5.md) -- chasquid configuration file

# SYNOPSIS

[chasquid.conf(5)](chasquid.conf.5.md) is [chasquid(1)](chasquid.1.md)'s main configuration file.

# DESCRIPTION

The file is in protocol buffers' text format.

Comments start with `#`. Empty lines are allowed.  Values are of the form
`key: value`. Values can be strings (quoted), integers, or booleans (`true` or
`false`).

Some values might be repeated, for example the listening addresses.

# OPTIONS

- **hostname** (string):

    Default hostname to use when saying hello. This is used to say hello to
    clients (for aesthetic purposes), and as the HELO/EHLO domain on outgoing SMTP
    connections (so ideally it would resolve back to the server, but it isn't a
    big deal if it doesn't). Default: the system's hostname.

- **max\_data\_size\_mb** (int):

    Maximum email size, in megabytes. Default: 50.

- **smtp\_address** (repeated string):

    Addresses to listen on for SMTP (usually port 25). Default: "systemd", which
    means systemd passes sockets to us. systemd sockets must be named with
    **FileDescriptorName=smtp**.

- **submission\_address** (repeated string):

    Addresses to listen on for submission (usually port 587). Default: "systemd",
    which means systemd passes sockets to us. systemd sockets must be named with
    **FileDescriptorName=submission**.

- **submission\_over\_tls\_address** (repeated string):

    Addresses to listen on for submission-over-TLS (usually port 465). Default:
    "systemd", which means systemd passes sockets to us. systemd sockets must be
    named with **FileDescriptorName=submission\_tls**.

- **monitoring\_address** (string):

    Address for the monitoring HTTP server. Do NOT expose this to the public
    internet. Default: no monitoring server.

- **mail\_delivery\_agent\_bin** (string):

    Mail delivery agent (MDA, also known as LDA) to use. This should point
    to the binary to use to deliver email to local users. The content of the
    email will be passed via stdin. If it exits unsuccessfully, we assume
    the mail was not delivered. Default: `maildrop`.

- **mail\_delivery\_agent\_args** (repeated string):

    Command line arguments for the mail delivery agent. One per argument.
    Some replacements will be done.

    On an email sent from marsnik@mars to venera@venus:

        %from%        -> from address (marsnik@mars)
        %from_user%   -> from user (marsnik)
        %from_domain% -> from domain (mars)
        %to%          -> to address (venera@venus)
        %to_user%     -> to user (venera)
        %to_domain%   -> to domain (venus)

    Default: `"-f", "%from%", "-d", "%to_user%"`  (adequate for procmail and
    maildrop).

- **data\_dir** (string):

    Directory where we store our persistent data. Default:
    `/var/lib/chasquid`.

- **suffix\_separators** (string):

    Suffix separator, to perform suffix removal of local users.  For
    example, if you set this to `-+`, email to local user `user-blah` and
    `user+blah` will be delivered to `user`.  Including `+` is strongly
    encouraged, as it is assumed for email forwarding.  Default: `+`.

- **drop\_characters** (string):

    Characters to drop from the user part on local emails.  For example, if
    you set this to `._`, email to local user `u.se_r` will be delivered to
    `user`.  Default: `.`.

- **mail\_log\_path** (string):

    Path where to write the mail log to.  If `<syslog>`, log using the
    syslog (at `MAIL|INFO` priority).  If `<stdout>`, log to stdout; if
    `<stderr>`, log to stderr.  Default: `<syslog>`.

- **dovecot\_auth** (bool):

    Enable dovecot authentication. If true, users that are not found in chasquid's
    databases will be authenticated via dovecot.  Default: `false`.

    The path to dovecot's auth sockets is autodetected, but can be manually
    overridden using the `dovecot_userdb_path` and `dovecot_client_path` if
    needed.

- **haproxy\_incoming** (bool):

    **EXPERIMENTAL**, might change in backwards-incompatible ways.

    If true, expect incoming SMTP connections to use the HAProxy protocol.
    This allows deploying chasquid behind a HAProxy server, as the address
    information is preserved, and SPF checks can be performed properly.
    Default: `false`.

- **max\_queue\_items** (int):

    **EXPERIMENTAL**, might change in backwards-incompatible ways.

    Maximum number of items in the queue.

    If we have this many items in the queue, we reject new incoming email. Be
    careful when increasing this, as we keep all items in memory.
    Default: `200` (but may change in the future).

- **give\_up\_send\_after** (string):

    **EXPERIMENTAL**, might change in backwards-incompatible ways.

    How long do we keep retrying sending an email before we give up.  Once we give
    up, a DSN will be sent back to the sender.

    The format is a Go duration string (e.g. "48h" or "360m"; note days are not a
    supported unit). Default: `"20h"` (but may change in the future).

# SEE ALSO

[chasquid(1)](chasquid.1.md)
