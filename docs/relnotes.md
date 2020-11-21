
# Release notes

This file contains notes for each release, summarizing changes and explicitly
noting backward-incompatible changes or known security issues.


## 1.6 (2020-11-22)

- Pass the EHLO domain to the post-data hook.
- Add /exit endpoint to monitoring server.
- Implement HAProxy protocol support (experimental).
- Documentation updates.


## 1.5 (2020-09-12)

- Add OpenMetrics exporter (compatible with Prometheus).
- Support log rotation via SIGHUP, and other misc. logging improvements.
- Fix error code on transient authentication issues.
- Fix rspamd greylist action handling in the default hook.
- Miscellaneous monitoring server improvements.


## 1.4 (2020-05-22)

- Use the configured hostname in outgoing SMTP HELO/EHLO.
- Allow config overrides from the command line.
- Miscellaneous test improvements and code cleanups.


## 1.3 (2020-04-12)

- Improved handling of DNS temporary errors.
- Documentation updates (use of SRS when forwarding, Dovecot troubleshooting,
  Arch installation instructions).
- Miscellaneous test improvements and cleanups.


## 1.2 (2019-12-06)

Security fixes:

- DoS through memory exhaustion due to not limiting the line length (on both
  incoming and outgoing connections). Thanks to Max Mazurov
  (fox.cpp@disroot.org) for the initial report.

Release notes:

- Fix handling of excessive long lines on incoming and outgoing connections.
- Better error codes when DATA size exceeded the maximum.
- New documentation sections (monitoring, release notes).
- Many miscellaneous test improvements.


## 1.1 (2019-10-26)

- Added hooks for aliases resolution.
- Added rspamd integration in the default post-data hook.
- Added chasquid-util aliases-add subcommand.
- Expanded SPF support.
- Documentation and test improvements.
- Minor bug fixes.


## 1.0 (2019-07-15)

No backwards-incompatible changes. No more are expected within this major
version.

- Fixed a bug on early connection deadline handling.
- Make DSN tidier, especially in handling multi-line errors.
- Miscellaneous test improvements.


## 0.07 (2019-01-19)

No backwards-incompatible changes.

- Send enhanced status codes.
- Internationalized Delivery Status Notifications (DSN).
- Miscellaneous test improvements.
- DKIM integration examples and test.


## 0.06 (2018-07-22)

No backwards-incompatible changes.

- New MTA-STS (Strict Transport Security) checking.


## 0.05 (2018-06-05)

No backwards-incompatible changes.

- Lots of new tests.
- Added a how-to and manual pages.
- Periodic reload of domaininfo, support removing entries manually.
- Dovecot auth support no longer considered experimental.


## 0.04 (2018-02-10)

No backwards-incompatible changes.

- Add Dovecot authentication support (experimental).
- Miscellaneous bug fixes to mda-lmtp and tests.


## 0.03 (2017-07-15)

**Backwards-incompatible changes:**

- The default MTA binary has changed. It's now maildrop by default.
  If you relied on procmail being the default, add the following to
  `/etc/chasquid/chasquid.conf`: `mail_delivery_agent_bin: "procmail"`.
- chasquid now listens on a third port, submission-on-TLS.
  If using systemd, copy the `etc/systemd/system/chasquid-submission_tls.socket`
  file to `/etc/systemd/system/`, and start it.


Release notes:

- Support submission (directly) over TLS (submissions/smtps/port 465).
- Change the default MDA binary to `maildrop`.
- Add a very basic MDA that uses LMTP to do the mail delivery.


## 0.02 (2017-03-03)

No backwards-incompatible changes.

- Improved configuration checks and safeguards.
- Fall back through the MX list on errors.
- Experimental MTA-STS implementation (disabled by default).


## 0.01 (2016-11-03)

Initial release.
