
This file contains notes for upgrading between different versions.


## 0.07 → 1.0

No backwards-incompatible changes. No more are expected within this major
version.


## 0.06 → 0.07

No backwards-incompatible changes.


## 0.05 → 0.06

No backwards-incompatible changes.


## 0.04 → 0.05

No backwards-incompatible changes.


## 0.03 → 0.04

No backwards-incompatible changes.


## 0.02 → 0.03

* The default MTA binary has changed. It's now maildrop by default.
  If you relied on procmail being the default, add the following to
  /etc/chasquid/chasquid.conf: `mail_delivery_agent_bin: "procmail"`.

* chasquid now listens on a third port, submission-on-TLS.
  If using systemd, copy the `etc/systemd/system/chasquid-submission_tls.socket`
  file to `/etc/systemd/system/`, and start it.

