
# Hooks

chasquid supports some functionality via hooks, which are binaries that get
executed at specific points in time during delivery.

They are optional, and will be skipped if they don't exist.


## Post-DATA hook

After completion of DATA, but before accepting the mail for queueing, chasquid
will run the command at `$config_dir/hooks/post-data`.

The contents of the mail will be written to the command's stdin, and the
environment is detailed below.

If the exit status is 0, chasquid will move forward processing the command,
and its stdout should contain headers which will be added to contents of
the email (at the top).

Otherwise, chasquid will respond with an error, and the last line of stdout
will be passed back to the client as the error message.
If the exit status is 20 the error code will be permanent, otherwise it will
be temporary.


This hook can be used to block based on contents, for example to check for
spam or virus.  See `etc/hooks/post-data` for an example.


### Environment

This hook will run as the chasquid user, so be careful about permissions and
privileges.

The environment will contain the following variables:

 - `$USER`: The server's `$USER` env variable.
 - `$SHELL`: The server's `$SHELL` env variable.
 - `$PATH`: The server's `$PATH` env variable.
 - `$PWD`: The working directory, which will be the config directory.
 - `$REMOTE_ADDR`: IP address of the remote side of the connection.
 - `$EHLO_DOMAIN`: EHLO/HELO domain, as given by the client; sanitized for
   safety.
 - `$EHLO_DOMAIN_RAW`: Same as `$EHLO_DOMAIN`, but not sanitized; be careful as
   it can contain problematic characters.
 - `$MAIL_FROM`: MAIL FROM address.
 - `$RCPT_TO`: RCPT TO addresses, space separated.
 - `$AUTH_AS`: Authenticated user; empty if the connection has not
   authenticated successfully.
 - `$ON_TLS`: 1 if using TLS, 0 if not.
 - `$FROM_LOCAL_DOMAIN`: 1 if the mail comes from a local domain, 0 if not.
 - `$SPF_PASS`: 1 if it passed SPF, 0 if not.

There is a 1 minute timeout for hook execution.
It will be run at the config directory.


## Alias resolve hook

When an alias needs to be resolved, chasquid will run the command at
`$config_dir/hooks/alias-resolve` (if the file exists).

The address to resolve will be passed as the single argument.

The output of the command will be parsed as if it was the right-hand side of
the aliases configuration file (see [Aliases](aliases.md) for more details).
Results are appended to the results of the file-based alias resolution.

If there is no alias for the address, the hook should just exit successfuly
without emitting any output.

There is a 5 second timeout for hook execution. If the hook exits with an
error, including timeout, delivery will fail.


## Alias exists hook

When chasquid needs to check whether an alias exists or not, it will run the
command at `$config_dir/hooks/alias-exists` (if the file exists).

The address to check will be passed as the single argument.

If the commands exits successfuly (exit code 0), then the alias exists; any
other exit code signals that the alias does not exist.

There is a 5 second timeout for hook execution. If the hook times out, the
alias will be assumed not to exist.
