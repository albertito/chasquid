
# Dovecot integration

As of version 0.04 (2018-02), [chasquid] has integration with [dovecot] for
authenticating users.

This means that chasquid can ask dovecot to authenticate users, instead/in
addition to having its own per-domain user databases.


## Configuring dovecot

The following needs to be added to the Dovecot configuration, usually in
`/etc/dovecot/conf.d/10-master.conf`:

```
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
```

If chasquid is running under a different user, adjust the `user = ` lines
accordingly.

This lets chasquid issue authentication requests to dovecot.

Authentication requests sent by chasquid to dovecot will use the
fully-qualified user form, `user@domain`.


## Configuring chasquid

Add the following line to `/etc/chasquid/chasquid.conf`:

```
dovecot_auth: true
```

That should be it, because chasquid will "autodetect" the full path to the
dovecot sockets, by looking in the usual places (tested in Debian, Ubuntu, and
CentOS).

If chasquid can't find them, the paths can be set with the
`dovecot_userdb_path` and `dovecot_client_path` options.


[dovecot]: https://dovecot.org
[chasquid]: https://blitiri.com.ar/p/chasquid
