
# Installing and configuring [chasquid](https://blitiri.com.ar/p/chasquid)

## Installation

### Debian/Ubuntu

If you're using [Debian](https://packages.debian.org/chasquid) or
[Ubuntu](https://packages.ubuntu.com/chasquid), chasquid can be installed by
running:

```shell
sudo apt install chasquid
```

### Arch

If you're using Arch, there is a
[chasquid AUR package](https://aur.archlinux.org/packages/chasquid/) you can
use.  See the [official Arch
documentation](https://wiki.archlinux.org/index.php/Arch_User_Repository) for
how to install it.  If you use the [pacaur](https://github.com/E5ten/pacaur)
[helper](https://wiki.archlinux.org/index.php/AUR_helpers), you can just run:

```shell
pacaur -S chasquid
```

[Binary packages](https://foxcpp.dev/archlinux/README.txt) are also available,
courtesy of foxcpp.


### From source

To get, build and install from source, you will need a working
[Go](http://golang.org) environment.

```shell
# Get the code and build the binaries.
git clone https://blitiri.com.ar/repos/chasquid
cd chasquid
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

### Certificates

The certs/ directory layout matches the one from
[certbot](https://certbot.eff.org/),
[letsencrypt](https://letsencrypt.org)'s
default client, to make it easier to integrate.

A convenient way to set this up is:

1) Obtain TLS certificates using `certbot` as needed.
2) Symlink chasquid's `certs/` to `/etc/letsencrypt/live`:\
   `sudo ln -s /etc/letsencrypt/live/ /etc/chasquid/certs`
3) Give chasquid permissions to read the certificates:\
   `sudo setfacl -R -m u:chasquid:rX /etc/letsencrypt/{live,archive}`
4) Set up [automatic renewal] to restart chasquid when certificates are
   renewed.

Please see the [how-to guide](howto.md#tls-certificate) for more detailed
examples.

[automatic renewal]: https://eff-certbot.readthedocs.io/en/stable/using.html#setting-up-automated-renewal


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
* anti-spam using spamassassin or rspamd.
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

