#!/usr/bin/env python3
#
# Simple "netcat" implementation.
# Unfortunately netcat/nc is not that portable, so this contains a simple
# implementation which fits our needs.

import argparse
import threading
import smtplib
import socket
import sys

ap = argparse.ArgumentParser()
ap.add_argument("-z", action='store_true', help="scan for listening daemons")
ap.add_argument("host", help="host to connect to")
ap.add_argument("port", type=int, help="port to connect to")
args = ap.parse_args()

address = (args.host, args.port)

try:
	sock = socket.create_connection(address)
	fd = sock.makefile('rw', buffering=1, encoding="utf-8")
except OSError:
	# Exit quietly, like nc does.
	sys.exit(1)

if args.z:
	sys.exit(0)


# stdin -> socket in the background. Do a partial shutdown when done.
def stdin_to_sock():
	for line in sys.stdin:
		fd.write(line)
		fd.flush()

	try:
		sock.shutdown(socket.SHUT_WR)
	except OSError:
		pass

t1 = threading.Thread(target=stdin_to_sock, daemon=True)
t1.start()

# socket -> stdout in the foreground; if the socket closes, exit.
for line in fd:
	sys.stdout.write(line)
	sys.stdout.flush()

