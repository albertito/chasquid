
# Release notes

This file contains notes for each release, summarizing changes and explicitly
noting backward-incompatible changes or known security issues.


## 1.15.0 (2025-01-17)

- Exit if there's an error reading users/aliases files on startup.
- Log how many things were loaded for each domain.
- Add fail2ban filter configuration example.

### 1.15.1 (2025-03-30)

Implement a workaround for a Microsoft bug in TLS session ticket handling,
that is causing deliverability issues, and they are being too slow at fixing.

See this [chasquid issue](https://github.com/albertito/chasquid/issues/64),
this [Go issue](https://github.com/golang/go/issues/70232) and this
[Postfix thread](https://www.mail-archive.com/postfix-users@postfix.org/msg104308.html)
for more details.


## 1.14.0 (2024-04-21)

- Add built-in [DKIM](dkim.md) signing and verification.
- Rename `master` branch to `main`. Docker users pulling from the `master`
  docker label should update the label accordingly. No action is needed if
  using `latest`.
- Starting with this release, version numbers will be
  [SemVer](https://semver.org/)-compatible, to help integration with other
  software that expects it (e.g. [pkg.go.dev](https://pkg.go.dev/)).


## 1.13 (2023-12-24)

Security fixes:

- Strict CRLF enforcement in DATA contents, to prevent [SMTP smuggling
  attacks](https://www.postfix.org/smtp-smuggling.html)
  ([CVE-2023-52354](https://nvd.nist.gov/vuln/detail/CVE-2023-52354)). \
  [RFC5322](https://www.rfc-editor.org/rfc/rfc5322#section-2.3) and
  [RFC5321](https://www.rfc-editor.org/rfc/rfc5321#section-2.3.8) say
  that the only valid newline terminator in SMTP is CRLF. \
  When an invalid newline terminator is found in an incoming message, the
  connection is now aborted immediately (previous releases also accepted
  LF-terminated lines). \
  The MTA courier now uses CRLF-terminated lines (previous releases used
  LF-terminated lines).

Other changes:

- Add support for receive-only users.
- Reject empty listening addresses, to help prevent accidental
  misconfiguration. To prevent chasquid from listening, just comment out the
  entry in the config.
- `docker/add-user.sh`: Support getting email and password from env variables.


## 1.12 (2023-10-07)

- Support [aliases with drop characters and
  suffix separators](aliases.md#drop-characters-and-suffix-separators).
- Improved delivery on some low-level TLS negotiation errors.
- `smtp-check`: Add flag to specify local name.
- `chasquid-util`: `aliases-resolve` and `domaininfo-remove` subcommands now
  talk to the running server. That results in more authoritative answers, and
  performance improvements.
- `chasquid-util`: Remove `aliases-add` subcommand. This was an undocumented
  command that was added a while ago, and there is no need for it anymore.
- Handle symlinks under the `certs/` directory.


## 1.11 (2023-02-19)

- New tracing library for improved observability.
- Update [fuzz tests](tests.md#fuzz-tests) to the new standard infrastructure.

### 1.11.1 (2023-12-26)

Backport the security fixes from 1.13 (*Strict CRLF enforcement in DATA
contents*, fixes
[CVE-2023-52354](https://nvd.nist.gov/vuln/detail/CVE-2023-52354)).


## 1.10 (2022-09-01)

- Support [catch-all aliases](aliases.md#catch-all).
- Fix bug in Docker image with user-provided certificates.
- Miscellaneous test improvements.


## 1.9 (2022-03-05)

- Improve certificate validation logic in the SMTP courier.
- Remove `alias-exists` hook, and improve aliases resolution logic.
- Support `""` values for `drop_characters` and `suffix_separators` in the
  configuration file.


## 1.8 (2021-07-30)

- Stricter error checking to help prevent cross-protocol attacks
  (like [ALPACA](https://alpaca-attack.com/)).
- Allow authenticating users without an `@domain` part.
- Add integration for
  [chasquid-rspamd](https://github.com/Thor77/chasquid-rspamd) and
  [dkimpy](https://launchpad.net/dkimpy/) in the example hook.
- Add a `-to_puny` option to mda-lmtp, to punycode-encode addresses.
- Use `application/openmetrics-text` as content type in the openmetrics
  exporter.


## 1.7 (2021-05-31)

- chasquid-util no longer depends on the unmaintained docopt-go.
  If you relied on undocumented parsing behaviour before, your invocations may
  need adjustment.  In particular, `--a b` is no longer supported, and `--a=b`
  must be used instead.
- Improve handling of errors when talking to Dovecot for authentication.
- Fix handling of `hostname` option in the Docker image.
- Miscellaneous documentation and test improvements.


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
