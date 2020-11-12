// Package haproxy implements the handshake for the HAProxy client protocol
// version 1, as described in
// https://www.haproxy.org/download/1.8/doc/proxy-protocol.txt.
package haproxy

import (
	"bufio"
	"errors"
	"net"
	"strconv"
	"strings"
)

var (
	errInvalidProtoID = errors.New("invalid protocol identifier")
	errUnkProtocol    = errors.New("unknown protocol")
	errInvalidFields  = errors.New("invalid number of fields")
	errInvalidSrcIP   = errors.New("invalid src ip")
	errInvalidDstIP   = errors.New("invalid dst ip")
	errInvalidSrcPort = errors.New("invalid src port")
	errInvalidDstPort = errors.New("invalid dst port")
)

// Handshake performs the HAProxy protocol v1 handshake on the given reader,
// which is expected to be backed by a network connection.
// It returns the source and destination addresses, or an error if the
// handshake could not complete.
// Note that any timeouts or limits must be set by the caller on the
// underlying connection, this is helper only to perform the handshake.
func Handshake(r *bufio.Reader) (src, dst net.Addr, err error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, nil, err
	}

	fields := strings.Fields(line)

	if len(fields) < 2 || fields[0] != "PROXY" {
		return nil, nil, errInvalidProtoID
	}

	switch fields[1] {
	case "TCP4", "TCP6":
		// Allowed to continue, nothing to do.
	default:
		return nil, nil, errUnkProtocol
	}

	if len(fields) != 6 {
		return nil, nil, errInvalidFields
	}

	srcIP := net.ParseIP(fields[2])
	if srcIP == nil {
		return nil, nil, errInvalidSrcIP
	}

	dstIP := net.ParseIP(fields[3])
	if dstIP == nil {
		return nil, nil, errInvalidDstIP
	}

	srcPort, err := strconv.ParseUint(fields[4], 10, 16)
	if err != nil {
		return nil, nil, errInvalidSrcPort
	}

	dstPort, err := strconv.ParseUint(fields[5], 10, 16)
	if err != nil {
		return nil, nil, errInvalidDstPort
	}

	src = &net.TCPAddr{IP: srcIP, Port: int(srcPort)}
	dst = &net.TCPAddr{IP: dstIP, Port: int(dstPort)}
	return src, dst, nil
}
