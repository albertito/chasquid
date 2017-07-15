
This file contains notes for upgrading between different versions.

As chasquid is still in beta, it is possible that some things change in
backwards-incompatible ways. This should be rare and will be avoided if
possible.


## 0.02 → 0.03

* The default MTA binary has changed. It's now maildrop by default.
  If you relied on procmail being the default, add the following to
  /etc/chasquid/chasquid.conf: `mail_delivery_agent_bin: "procmail"`.

* chasquid now listens on a third port, submission-on-TLS.
  If using systemd, copy the `etc/systemd/system/chasquid-submission_tls.socket`
  file to `/etc/systemd/system/`, and start it.


