# NAME

chasquid - SMTP (email) server

# SYNOPSIS

**chasquid** \[_options_...\]

# DESCRIPTION

chasquid is an SMTP (email) server with a focus on simplicity, security, and
ease of operation.

It's written in Go, and distributed under the Apache license 2.0.

# OPTIONS

- **-config\_dir** _dir_

    configuration directory (default `/etc/chasquid`)

- **-config\_overrides** _config_

    configuration values (in text protobuf format) to override the on-disk
    configuration with. This should only be needed in very specific cases for
    deployments where editing the configuration file is not feasible.

- **-alsologtostderr**

    also log to stderr, in addition to the file

- **-logfile** _file_

    file to log to (enables logtime)

- **-logtime**

    include the time when writing the log to stderr

- **-logtosyslog** _tag_

    log to syslog, with the given tag

- **-v** _level_

    verbosity level (1 = debug)

- **-version**

    show version and exit

# FILES

The daemon's configuration is by default in `/etc/chasquid/`, and can be
changed with the _-config\_dir_ flag.

Inside that directory, the daemon expects the following structure:

- `chasquid.conf`

    Main config file, see [chasquid.conf(5)](chasquid.conf.5.md).

- `domains/`

    Per-domain configuration.

- `domains/example.com/`

    Domain-specific configuration. Can be empty.

- `domains/example.com/users`

    User and password database for this domain.

- `domains/example.com/aliases`

    Aliases for the domain.

- `certs/`

    Certificates to use, one directory per pair.

- `certs/mx.example.com/`

    Certificates for this domain.

- `certs/mx.example.com/fullchain.pem`

    Certificate (full chain).

- `certs/mx.example.com/privkey.pem`

    Private key.

Note the `certs/` directory layout matches the one from certbot (client for
Let's Encrypt CA), so you can just symlink `certs/` to
`/etc/letsencrypt/live`.

Make sure the user you use to run chasquid under ("mail" in the example
config) can access the certificates and private keys.

# CONTACT

[Main website](https://blitiri.com.ar/p/chasquid).

If you have any questions, comments or patches please send them to the mailing
list, `chasquid@googlegroups.com`.  To subscribe, send an email to
`chasquid+subscribe@googlegroups.com`.

# SEE ALSO

[chasquid-util(1)](chasquid-util.1.md), [chasquid.conf(5)](chasquid.conf.5.md)
