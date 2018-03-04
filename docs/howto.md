
# Setting up an email server with Debian, dovecot and chasquid

This is a practical guide for setting up an email server for personal or small
groups use. It does not contain many explanations, but includes links to more
detailed references where possible.

While a lot of the contents are generic, for simplicity it will use:

 - [Debian] \(testing\) as base operating system ([Ubuntu] also works)
 - [Dovecot] for [POP3]+[IMAP]
 - [chasquid] for [SMTP]
 - [Let's Encrypt] for [TLS] certificates

[Debian]: https://debian.org
[Ubuntu]: https://ubuntu.com
[Dovecot]: https://dovecot.org
[chasquid]: https://blitiri.com.ar/p/chasquid
[Let's Encrypt]: https://letsencrypt.org
[POP3]: https://en.wikipedia.org/wiki/Post_Office_Protocol
[IMAP]: https://en.wikipedia.org/wiki/Internet_Message_Access_Protocol
[SMTP]: https://en.wikipedia.org/wiki/Simple_Mail_Transfer_Protocol
[TLS]: https://en.wikipedia.org/wiki/Transport_Layer_Security


## Example data

This guide will use the following data for illustration purposes, replace them
with your own where appropriate.

 - Domain name: `example.com`.
 - IPv4 address of your mail server: `198.51.100.7`.
 - IPv6 address of your mail server: `2001:db8::7`.

Note IPv6 is optional but highly encouraged, and supported by most providers.


## Getting a server

You first need to have a server to use. This could be an existing one (for
example, if you already have one where you host HTTP), doesn't have to be
exclusive for email.

In this guide we will use a separate server, mostly for clarity.

For small groups the size of the server does not matter, any small VPS
(virtual private server) will do just fine.

Specifically for hosting email servers, there are some things to check when
selecting a provider:

 - Make sure they allow traffic on TCP port 25 (SMTP). While almost all VPS
   and dedicated server providers are fine, some "cloud" providers (like
   Google Cloud) block port 25, which is used for sending and receiving mails.
 - Once you get a server, make sure the IP address is not listed in any
   [blackhole lists].
   There are many services to check them, for example the one from [the
   Anti-Abuse project].

Remember to update your server regularly, setting up [unattended upgrades] is
highly recommended.

[the Anti-Abuse project]: http://www.anti-abuse.org/multi-rbl-check/
[blackhole lists]: https://en.wikipedia.org/wiki/DNSBL
[unattended upgrades]: https://wiki.debian.org/UnattendedUpgrades


## DNS

Set up the following DNS records for `example.com`.  This is usually done
either in your DNS server, or in the user interface of your DNS provider.

```
; Assign "mail.example.com" to the server's IP addresses.
; Replace these with the ones for your server.
mail    A       198.51.100.7
mail    AAAA    2001:db8::7

; The mail server for example.com is mail.example.com.
@       MX      10  mail

; Use SPF to say that the servers in "MX" above are allowed to send email
; for this domain, and nobody else.
@       TXT     "v=spf1 mx -all"
```

Finally, you should go to your server provider and configure the "reverse DNS"
(also known as "PTR") for the IP addresses to be to "mail.example.com". This
is important, as some spam checkers will consider it a factor.

*References:
[A record](https://en.wikipedia.org/wiki/A_record),
[MX record](https://en.wikipedia.org/wiki/MX_record),
[Sender Policy Framework (SPF)](https://en.wikipedia.org/wiki/Sender_Policy_Framework).*


## TLS certificate

[TLS] certificates are needed to send and receive email securely.
[letsencrypt] will provide us with a free certificate, which needs to be
renewed every 90 days, so the following relies on automatic renewal.

Note `certbot` is the recommended letsencrypt command line client.

```shell
sudo apt install certbot acl

# Obtain a TLS certificate for mail.example.com.
sudo certbot certonly --standalone -d mail.example.com

# Give chasquid access to the certificates.
# Dovecot does not need this as it reads them as root.
sudo setfacl -R -m u:chasquid:rX /etc/letsencrypt/{live,archive}

# Automatically restart the daemons after each certificate renewal.
sudo mkdir -p /etc/letsencrypt/renewal-hooks/post
cat <<EOF | sudo tee /etc/letsencrypt/renewal-hooks/post/restart
#!/bin/bash

systemctl restart chasquid
systemctl restart dovecot
EOF
sudo chmod +x /etc/letsencrypt/renewal-hooks/post/restart
```

[TLS]: https://en.wikipedia.org/wiki/Transport_Layer_Security
[letsencrypt]: https://letsencrypt.org


## Configure dovecot

First, install dovecot, and let chasquid use it for authorizing users. That
way, you will only use a single system for managing users (dovecot).

```shell
sudo apt install dovecot-imapd dovecot-pop3d dovecot-lmtpd

cat <<EOF | sudo tee /etc/dovecot/conf.d/11-chasquid.conf
# Allow chasquid to authorize users via dovecot.
service auth {
  unix_listener auth-chasquid-userdb {
    mode = 0660
    user = chasquid
  }
  unix_listener auth-chasquid-client {
    mode = 0660
    user = chasquid
  }
}
EOF
```

You will need to configure dovecot authentication depending on your needs.
For example, if you want to use only system users, or virtual users.
See the `/etc/dovecot/conf.d/10-auth.conf` file, and the [dovecot
documentation](https://wiki.dovecot.org/HowTo/SimpleVirtualInstall) for more
details.


## Configure chasquid

```shell
sudo apt install chasquid
sudo setfacl -R -m u:chasquid:rX /etc/chasquid/

# Use the certificates obtained from certbot.
sudo mv /etc/chasquid/certs/ /etc/chasquid/certs-orig
sudo ln -s /etc/letsencrypt/live/ /etc/chasquid/certs

# Make chasquid accept mail for "example.com".
sudo mkdir -p /etc/chasquid/domains/example.com

# Tell chasquid to deliver local mails to dovecot, and use it for
# authentication.
cat <<EOF | sudo tee -a /etc/chasquid/chasquid.conf

# Deliver email via lmtp to dovecot.
mail_delivery_agent_bin: "/usr/local/bin/mda-lmtp"
mail_delivery_agent_args: "--addr"
mail_delivery_agent_args: "/run/dovecot/lmtp"
mail_delivery_agent_args: "-f"
mail_delivery_agent_args: "%from%"
mail_delivery_agent_args: "-d"
mail_delivery_agent_args: "%to_user%"

# Use dovecot authentication (only available in chasquid >= 0.04).
dovecot_auth: true
EOF
```

## Add users

With this configuration, chasquid will use dovecot to manage users, so refer
to the [dovecot documentation](https://wiki.dovecot.org/BasicConfiguration)
for the details.

You can also add chasquid-specific users with `chasquid-util add-user`.


## Additional domains

To make chasquid manage an additional domain `otherdomain.com`, first add the
following records to `otherdomain.com`:

```
@       MX      10  mail.example.com
@       TXT     "v=spf1 mx -all"
```

Then, tell chasquid about it by running `mkdir
/etc/chasquid/domains/otherdomain.com`. Don't forget to restart it afterwards.


Alternatively, you can use a different MX record, as long as you can get
chasquid a certificate for it.


## Optional software

- To use [SpamAssassin] to filter spam, run `apt install spamassassin spamc`.
- To use [ClamAV] to filter viruses, run `apt install clamdscan`.

That's all it takes. chasquid default hooks will pass incoming mail through
both if (and only if) they are installed.

[SpamAssassin]: https://spamassassin.apache.org/
[ClamAV]: https://www.clamav.net/

