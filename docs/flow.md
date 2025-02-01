
# Message flows

This document explains at a high level some parts of chasquid's message
processing, in particular how messages flow through the system.


## Message reception

- Client connects to chasquid on the smtp or submission ports, and issues
  HELO/EHLO.
- Client optionally performs STARTTLS.
- Client optionally performs AUTH.
    - Check that this is done over TLS.
- Client sends MAIL FROM.
    - Check SPF.
    - Check connection security level.
- Client sends one or more RCPT TO.
    - If the destination is remote, then the user must have authenticated.
    - If the destination is local, check that the user exists.
- Client sends DATA.
- Client sends actual data, and ends it with '.'
    - Parse the data contents to perform loop detection.
    - If the sender is authenticated, DKIM-sign the email with the
      corresponding key.
    - If the sender is not authenticated, verify the DKIM signature (if the
      email has one).
    - Add the required headers (Received, SPF results, post-data hook output).
    - Run the post-data hook. If the hook fails, return an error.
    - Put it in the queue and reply success.


### Authenticated mail, and email spoofing

By default, authenticated users can send emails as any other user or domain.
For example, you can authenticate as `a@a`, and send email as `b@b`.

This is a design choice made to balance simplicity of operation and use.

Users who want to be strict about "MAIL FROM" or even "From:" validation can
add additional checks in the [post-DATA hook](hooks.md).

In the future, chasquid may get some option to be strict about it by default,
or on a per-domain or per-user basis. But for now, using a [post-DATA
hook](hooks.md) is the best way to make chasquid more strict about this.


## Queue processing

Before accepting a message:

- Create a (pseudo) random internal ID for it.
- For each recipient, use the alias database to expand it, add the results to
  the list of final recipients (which may not be email).
- Save the resulting envelope (with the final recipients) to disk.

Queue processing runs asynchronously, there's a goroutine for each message
which does, in a loop:

- For each recipient which we have not delivered yet:
    - Attempt delivery.
    - Write to disk the results.
- If there are mails still pending, wait for some time (incrementally).
- When all the recipients have completed delivery, or enough time has passed:
    - If all were successful, remove from the queue.
    - If some failed, send a delivery status notification back to the sender.

