
# Known issues

This file contains a list of the most common known issues, and the release
range affected. It can be useful for people running older versions, to
identify problems (and workarounds) faster.

Entries are eventually be purged once their affected versions become uncommon,
to prevent confusion.


## Dovecot auth occasionally not functional after a reboot (0.04 to 1.6)

After a reboot, if chasquid starts *before* dovecot, it's possible that
chasquid fails to autodetect the dovecot addresses, and the dovecot
authentication will not be functional until chasquid is restarted.

This condition can be identified by seeing
`Dovecot autodetection failed, no dovecot fallback` in the chasquid logs, at
start-up time.

As a workaround, you can create the following systemd dropin file at
`/etc/systemd/system/chasquid.service.d/after-dovecot.conf`, to make chasquid
be started *after* dovecot:

```
[Unit]
After=dovecot.service
```

The issue is fixed in 1.7.


## `dkimsign` causes parsing errors in post-data hook (0.07 to 1.5)

The default post-data hook in versions 0.07 to 1.5 has a bug where if the
`dkimsign` command exists, unwanted output will be emitted and cause the
post-data hook invocation to fail.

The problem can be identified by the following error in the logs:

```
Hook.Post-DATA 1.2.3.4:5678: error: error parsing post-data output: \"/usr/bin/dkimsign\\n...
```

As a workaround, you can edit the hook and make the change
[seen here](https://blitiri.com.ar/git/r/chasquid/c/b6248f3089d7df93035bbbc0c11edf50709d5eb0/).

The issue is fixed in 1.6.
