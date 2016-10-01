#!/usr/bin/env python3
#
# Simple SMTP client for testing purposes.

import argparse
import email.parser
import email.policy
import smtplib
import sys

ap = argparse.ArgumentParser()
ap.add_argument("--server", help="SMTP server to connect to")
ap.add_argument("--user", help="Username to use in SMTP AUTH")
ap.add_argument("--password", help="Password to use in SMTP AUTH")
args = ap.parse_args()

# Parse the email using the "default" policy, which is not really the default.
# If unspecified, compat32 is used, which does not support UTF8.
msg = email.parser.Parser(policy=email.policy.default).parse(sys.stdin)

s = smtplib.SMTP(args.server)
s.starttls()
s.login(args.user, args.password)

# Note this does NOT support non-ascii message payloads transparently (headers
# are ok).
s.send_message(msg)
s.quit()


