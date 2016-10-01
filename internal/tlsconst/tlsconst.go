// Package tlsconst contains TLS constants for human consumption.
package tlsconst

// Most of the constants get automatically generated from IANA's assignments.
//go:generate ./generate-ciphers.py ciphers.go

import "fmt"

var versionName = map[uint16]string{
	0x0300: "SSL-3.0",
	0x0301: "TLS-1.0",
	0x0302: "TLS-1.1",
	0x0303: "TLS-1.2",
}

// VersionName returns a human-readable TLS version name.
func VersionName(v uint16) string {
	name, ok := versionName[v]
	if !ok {
		return fmt.Sprintf("TLS-%#04x", v)
	}
	return name
}

// CipherSuiteName returns a human-readable TLS cipher suite name.
func CipherSuiteName(s uint16) string {
	name, ok := cipherSuiteName[s]
	if !ok {
		return fmt.Sprintf("TLS_UNKNOWN_CIPHER_SUITE-%#04x", s)
	}
	return name
}
