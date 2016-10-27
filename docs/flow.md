
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
    - Run the post-data hook. If the hook fails, return an error.
    - Parse the data contents to perform loop detection.
    - Add the required headers (Received, SPF results, post-data hook output).
    - Put it in the queue and reply success.


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

