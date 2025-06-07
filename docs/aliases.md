
# Aliases

[chasquid] supports [email aliases], which is a mechanism to redirect mail
from one account to others.


## File format

The aliases are configured per-domain, on a text file named `aliases` within
the domain directory. So like `/etc/chasquid/domains/example.com/aliases`.

The format is very similar to the one used by classic MTAs (sendmail, exim,
postfix), but not identical.

### Comments

Lines beginning with `#` are considered comments, and are ignored.

### Email aliases

To create email aliases, where mail to a user are redirected to other
addresses, write lines of the form `user: address, address, ...`.

The user should not have the domain specified, as it is implicit by the
location of the file. The domain in target addresses is optional, and defaults
to the user domain if not present.

For example:

```
# Redirect mail to pepe@ to jose@ on the same domain.
pepe: jose

# Redirect mail to flowers@ to the individual flowers.
flowers: rose@backgarden, lilly@pond
```

Destination addresses can be for a remote domain as well. In that case, the
email will be forwarded using
[sender rewriting](https://en.wikipedia.org/wiki/Sender_Rewriting_Scheme).
While the content of the message will not be changed, the envelope sender will
be the constructed from the alias user.

User names cannot contain spaces, ":" or commas, for parsing reasons. This is
a tradeoff between flexibility and keeping the file format easy to edit for
people. User names will be normalized internally to lower-case. UTF-8 is
allowed and fully supported.

### Pipe aliases

A pipe alias is of the form `user: | command`, and causes mail to be sent as
standard input to the given command.

The command can have space-separated arguments (no quotes or escaping
expansion will be performed).

For example:

```
# Mail to user@ will be piped to this command for delivery.
user: | /usr/bin/email-handler --work

# Mail to null@ will be piped to "cat", effectively discarding the email.
null: | cat
```

### Catch-all

If the aliased user is `*`, then mail sent to an unknown user will not be
rejected, but redirected to the indicated destination instead.

```
pepe: jose

*: pepe, rose@backgarden
```

!!! warning "Experimental"

    If the destination address has `*` as its user, then it will be replaced
    by the sender user. Note that in this case, the user is copied as-is, no
    characters or suffixes will be dropped.

    For example, `*: *@pond` will redirect `lilly@domain` to `lilly@pond`.

    This is experimental as of chasquid 1.16.0, and subject to change.

### "Via" aliases (experimental) {#via}

!!! warning

    This feature is experimental as of chasquid 1.16.0, and subject to change.

A "via" alias is like an email alias, but it explicitly specifies which
server(s) to use when delivering that email. The servers are used to attempt
delivery in the given order.

This can be useful in scenarios such as secondary MX servers that forward all
email to the primary server, or send-only servers.

The syntax is `user: address via server1[/server2/...]`.

Examples:

```
# Emails sent to pepe@domain will be forwarded to jose@domain using
# mail.example.com (instead of the MX records of the domain).
pepe: jose via mail1.example.com

# Same as above, but with multiple servers. They will be tried in order.
flowers: lilly@pond via mail1.pond/mail2.pond

# Forward all email (that does not match other users or aliases) using
# mail1.example.com.
# This is a typical setup for a secondary MX server that forwards email to
# the primary.
*: * via mail1.example.com
```


### Overrides

If the same left-side address appears more than once, the last one will take
precedence.

For example, in this case, the result is that `pepe` is aliased to `jose`, the
first line is effectively ignored.

```
pepe: juan
pepe: jose
```

### Drop characters and suffix separators

When parsing aliases files, drop characters will be ignored. Suffix separators
are kept as-is.

When doing lookups, drop characters will also be ignored. If the address has a
suffix, the lookup will include it; if there is no match, it will try again
without the suffix.

In practice, this means that if the aliases file contains:

```
juana.perez: juana
juana.perez+fruta: fruta
```

Then (assuming the default drop characters and suffix separators), these are
the results:

```
juana.perez -> juana
juanaperez -> juana
ju.ana.pe.rez -> juana

juana.perez+abc -> juana
juanaperez+abc -> juana

juana.perez+fruta -> fruta
juanaperez+fruta -> fruta
```

This allows addresses with suffixes to have specific aliases, without having
to worry about drop characters, which is the most common use case.

If different semantics are needed, they can be implemented using the
[hook](#hooks).


## Processing

Aliases files are read upon start-up and refreshed every 30 seconds, so
changes to them don't require a daemon restart.

The resolver will perform lookups recursively, until it finds all the final
recipients. There are recursion limits to avoid alias loops. If the limit (10
levels) is reached, the entire resolution will fail.

Commands are given 30s to run, after which it will be killed and the execution
will fail.  If the command exits with an error (non-0 exit code), the delivery
will be considered failed.

The `chasquid-util` command-line tool can be used to check and resolve
aliases manually. It talks to the running server, so the response is fully
authoritative.


## Hooks

There is a hook that allows more sophisticated aliases resolution:
`alias-resolve`.

If it exists, it is invoked as part of the resolution process, and the results
are merged with the file-based resolution results.

See the [hooks](hooks.md) documentation for more details.


[chasquid]: https://blitiri.com.ar/p/chasquid
[email aliases]: https://en.wikipedia.org/wiki/Email_alias
