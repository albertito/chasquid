
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



## Installation

If you're using Debian or Ubuntu, chasquid can be installed by running
`sudo apt install chasquid`.

To get the code and build it, you will need a working [Go](http://golang.org)
environment.

```shell
# Get the code and build the binaries.
go get blitiri.com.ar/go/chasquid
cd "$GOPATH/src/blitiri.com.ar/go/chasquid"
make

# Install the binaries to /usr/local/bin.
sudo make install-binaries

# Copy the example configuration to /etc/chasquid and /etc/systemd, and create
# the /var/lib/chasquid directory.
sudo make install-config-skeleton
```

## Configuration

The configuration is in `/etc/chasquid/` by default, and has the following
structure:

```
- chasquid.conf      Main config file.

- domains/           Domains' data.
  - example.com/
    - users          User and password database for the domain.
    - aliases        Aliases for the domain.
  ...

- certs/             Certificates to use, one dir per pair.
  - mx.example.com/
    - fullchain.pem  Certificate (full chain).
    - privkey.pem    Private key.
  ...
```

Note the certs/ directory matches certbot's structure, so if you use it you
can just symlink to /etc/letsencrypt/live.

Make sure the user you use to run chasquid under ("mail" in the example
config) can access the certificates and private keys.


### Adding users

You can add users with:

```
chasquid-util user-add user@domain
```

This will also create the corresponding domain directory if it doesn't exist.


### Checking your configuration

Run `chasquid-util print-config` to parse your configuration and display the
resulting values.


### Checking your setup

Run `smtp-check yourdomain.com`, it will check:

* MX DNS records.
* SPF DNS records (will just warn if not present).
* TLS certificates.

It needs to access port 25, which is often blocked by ISPs, so it's likely
that you need to run it from your server.


### Greylisting, anti-spam and anti-virus

chasquid supports running a post-DATA hook, which can be used to perform
greylisting, and run anti-spam and anti-virus filters.

The hook should be at `/etc/chasquid/hooks/post-data`.


The one installed by default is a bash script supporting:

* greylisting using greylistd.
* anti-spam using spamassassin.
* anti-virus using clamav.

To use them, they just need to be available in your system.

For example, in Debian you can run the following to install all three:

```
apt install greylistd spamc clamdscan
usermod -a -G greylist mail
```

Note that the default hook may not work in all cases, it is provided as a
practical example but you should adjust it to your particular system if
needed.

