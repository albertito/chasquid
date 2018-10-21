#!/usr/bin/env python3
#
# Simple SMTP client for testing purposes.

import argparse
import email.parser
import email.policy
import re
import smtplib
import sys

ap = argparse.ArgumentParser()
ap.add_argument("--server", help="SMTP server to connect to")
ap.add_argument("--user", help="Username to use in SMTP AUTH")
ap.add_argument("--password", help="Password to use in SMTP AUTH")
args = ap.parse_args()

# Parse the email using the "default" policy, which is not really the default.
# If unspecified, compat32 is used, which does not support UTF8.
rawmsg = sys.stdin.buffer.read()
msg = email.parser.Parser(policy=email.policy.default).parsestr(
        rawmsg.decode('utf8'))

s = smtplib.SMTP(args.server)
s.starttls()
if args.user:
    s.login(args.user, args.password)

# Send the raw message, not parsed, because the parser does not handle some
# corner cases that well (for example, DKIM-Signature headers get mime-encoded
# incorrectly).
# Replace \n with \r\n, which is normally done by the library, but will not do
# it in this case because we are giving it bytes and not a string (which we
# cannot do because it tries to incorrectly escape the headers).
crlfmsg = re.sub(br'(?:\r\n|\n|\r(?!\n))', b"\r\n", rawmsg)

s.sendmail(
        from_addr=msg['from'], to_addrs=msg.get_all('to'),
        msg=crlfmsg,
        mail_options=['SMTPUTF8'])
s.quit()


