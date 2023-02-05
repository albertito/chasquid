// Fuzz testing for package smtpsrv.  Based on server_test.
package smtpsrv

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"net"
	"net/textproto"
	"strings"
	"testing"
)

func fuzzConnection(t *testing.T, modeI int, data []byte) {
	var mode SocketMode
	addr := ""
	switch modeI {
	case 0:
		mode = ModeSMTP
		addr = smtpAddr
	case 1:
		mode = ModeSubmission
		addr = submissionAddr
	case 2:
		mode = ModeSubmissionTLS
		addr = submissionTLSAddr
	default:
		mode = ModeSMTP
		addr = smtpAddr
	}

	var err error
	var conn net.Conn
	if mode.TLS {
		conn, err = tls.Dial("tcp", addr, tlsConfig)
	} else {
		conn, err = net.Dial("tcp", addr)
	}
	if err != nil {
		panic(fmt.Errorf("failed to dial: %v", err))
	}
	defer conn.Close()

	tconn := textproto.NewConn(conn)
	defer tconn.Close()

	scanner := bufio.NewScanner(bytes.NewBuffer(data))
	for scanner.Scan() {
		line := scanner.Text()
		cmd := strings.TrimSpace(strings.ToUpper(line))

		// Skip STARTTLS if it happens on a non-TLS connection - the jump is
		// not going to happen via fuzzer, it will just cause a timeout (which
		// is considered a crash).
		if cmd == "STARTTLS" && !mode.TLS {
			continue
		}

		if err = tconn.PrintfLine(line); err != nil {
			break
		}

		if _, _, err = tconn.ReadResponse(-1); err != nil {
			break
		}

		if cmd == "DATA" {
			// We just sent DATA and got a response; send the contents.
			err = exchangeData(scanner, tconn)
			if err != nil {
				break
			}
		}
	}
}

func FuzzConnection(f *testing.F) {
	f.Fuzz(fuzzConnection)
}

func exchangeData(scanner *bufio.Scanner, tconn *textproto.Conn) error {
	for scanner.Scan() {
		line := scanner.Text()
		if err := tconn.PrintfLine(line); err != nil {
			return err
		}
		if line == "." {
			break
		}
	}

	// Read the "." response.
	_, _, err := tconn.ReadResponse(-1)
	return err
}
