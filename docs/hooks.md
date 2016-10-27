
# Post-DATA hook

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


## Environment

This hook will run as the chasquid user, so be careful about permissions and
privileges.

The environment will contain the following variables:

 - USER
 - SHELL
 - PATH
 - PWD
 - REMOTE_ADDR
 - MAIL_FROM
 - RCPT_TO (space separated)
 - AUTH_AS (empty if not completed AUTH)
 - ON_TLS (0 if not, 1 if yes)
 - FROM_LOCAL_DOMAIN (0 if not, 1 if yes)
 - SPF_PASS (0 if not, 1 if yes)

There is a 1 minute timeout for hook execution.
It will be run at the config directory.

