
# HAProxy integration

As of version 1.6, [chasquid] supports being deployed behind a [HAProxy]
instance.

**This is EXPERIMENTAL for now, and can change in backwards-incompatible
ways.**


## Configuring HAProxy

In the backend server line, set the [send-proxy] parameter to turn on the use
of the PROXY protocol against chasquid.

You need to set this for each of the ports that are forwarded.


## Configuring chasquid

Add the following line to `/etc/chasquid/chasquid.conf`:

```
haproxy_incoming: true
```

That turns HAProxy support on for all incoming SMTP connections.


[chasquid]: https://blitiri.com.ar/p/chasquid
[HAProxy]: https://www.haproxy.org/
[send-proxy]: http://cbonte.github.io/haproxy-dconv/2.0/configuration.html#5.2-send-proxy
