#!/usr/bin/env python3
#
# This hacky script generates a go file with a map of version -> name for the
# entries in the TLS Cipher Suite Registry.

import csv
import urllib.request
import sys

# Where to get the TLS parameters from.
# See https://www.iana.org/assignments/tls-parameters/tls-parameters.xhtml.
URL = "https://www.iana.org/assignments/tls-parameters/tls-parameters-4.csv"


def getCiphers():
	req = urllib.request.urlopen(URL)
	data = req.read().decode('utf-8')

	ciphers = []
	reader = csv.DictReader(data.splitlines())
	for row in reader:
		desc = row["Description"]
		rawval = row["Value"]

		# Just plain TLS values for now, to keep it simple.
		if "-" in rawval or not desc.startswith("TLS"):
			continue

		rv1, rv2 = rawval.split(",")
		rv1, rv2 = int(rv1, 16), int(rv2, 16)

		val = "0x%02x%02x" % (rv1, rv2)
		ciphers.append((val, desc))

	return ciphers


ciphers = getCiphers()

out = open(sys.argv[1], 'w')
out.write("""\
package tlsconst

// AUTOGENERATED - DO NOT EDIT
//
// This file was autogenerated by generate-ciphers.py.

var cipherSuiteName = map[uint16]string{
""")

for ver, desc in ciphers:
	out.write('\t%s: "%s",\n' % (ver, desc))

out.write('}\n')
