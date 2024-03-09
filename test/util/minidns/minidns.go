//go:build !coverage
// +build !coverage

// minidns is a trivial DNS server used for testing.
//
// It takes an "answers" file which contains lines with the following format:
//
//	<domain> <type> <value>
//
// For example:
//
//	blah A  1.2.3.4
//	blah MX mx1
//
// Supported types: A, AAAA, MX, TXT.
//
// It's only meant to be used for testing, so it's not robust, performant, or
// standards compliant.
package main

import (
	"bufio"
	"encoding/binary"
	"flag"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"

	"blitiri.com.ar/go/log"
	"golang.org/x/net/dns/dnsmessage"
)

var (
	addr      = flag.String("addr", ":53", "address to listen to (UDP)")
	zonesPath = flag.String("zones", "", "file with the zones")
)

func main() {
	flag.Parse()

	srv := &miniDNS{
		answers: map[string][]dnsmessage.Resource{},
	}

	if *zonesPath == "" {
		log.Fatalf("-zones must be given")
	}
	var zonesFile *os.File
	if *zonesPath == "-" {
		zonesFile = os.Stdin
	} else {
		var err error
		zonesFile, err = os.Open(*zonesPath)
		if err != nil {
			log.Fatalf("error opening %v: %v", *zonesPath, err)
		}
	}

	srv.loadZones(zonesFile)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		srv.listenAndServeUDP(*addr)
	}()
	go func() {
		defer wg.Done()
		srv.listenAndServeTCP(*addr)
	}()
	wg.Wait()
}

type miniDNS struct {
	// Domain -> Answers.
	// We always respond the same regardless of the query.
	// Not great, but does the trick.
	answers map[string][]dnsmessage.Resource
}

func (m *miniDNS) listenAndServeUDP(addr string) {
	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		log.Fatalf("error listening UDP %q: %v", addr, err)
	}

	log.Infof("listening on %v", conn.LocalAddr())

	buf := make([]byte, 64*1024)
	for {
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			log.Infof("error reading from udp: %v", err)
			continue
		}

		msg := &dnsmessage.Message{}
		err = msg.Unpack(buf[:n])
		if err != nil {
			log.Infof("%v error unpacking message: %v", addr, err)
		}

		if lq := len(msg.Questions); lq != 1 {
			log.Infof("%v/%-5d  dropping packet with %d questions",
				addr, msg.ID, lq)
			continue
		}
		q := msg.Questions[0]
		log.Infof("%v/%-5d   Q: %s %s %s",
			addr, msg.ID, q.Name, q.Type, q.Class)

		reply := m.handle(msg)
		rbuf, err := reply.Pack()
		if err != nil {
			log.Fatalf("error packing reply: %v", err)
		}

		_, err = conn.WriteTo(rbuf, addr)
		if err != nil {
			log.Infof("%v/%-5d  error writing: %v",
				addr, msg.ID, err)
		}
	}
}

func (m *miniDNS) listenAndServeTCP(addr string) {
	ls, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("error listening TCP %q: %v", addr, err)
	}

	log.Infof("listening on %v", addr)

	for {
		conn, err := ls.Accept()
		if err != nil {
			log.Infof("error accepting: %v", err)
			continue
		}

		msg, err := readTCPMessage(conn)
		if err != nil {
			log.Infof("%v error reading message: %v", addr, err)
			conn.Close()
			continue
		}

		if lq := len(msg.Questions); lq != 1 {
			log.Infof("%v/%-5d  dropping packet with %d questions",
				addr, msg.ID, lq)
			conn.Close()
			continue
		}
		q := msg.Questions[0]
		log.Infof("%v/%-5d   Q: %s %s %s",
			addr, msg.ID, q.Name, q.Type, q.Class)

		reply := m.handle(msg)
		err = writeTCPMessage(conn, reply)
		if err != nil {
			log.Infof("error writing reply: %v", err)
		}

		conn.Close()
	}
}

func readTCPMessage(conn net.Conn) (*dnsmessage.Message, error) {
	// Read the 2-byte length first, then the message.
	lenHdr := struct{ Len uint16 }{}
	err := binary.Read(conn, binary.BigEndian, &lenHdr)
	if err != nil {
		return nil, err
	}

	data := make([]byte, lenHdr.Len)
	err = binary.Read(conn, binary.BigEndian, &data)
	if err != nil {
		return nil, err
	}

	msg := &dnsmessage.Message{}
	err = msg.Unpack(data)
	if err != nil {
		return nil, fmt.Errorf("%v error unpacking message: %v", addr, err)
	}

	return msg, nil
}

func writeTCPMessage(conn net.Conn, msg *dnsmessage.Message) error {
	rbuf, err := msg.Pack()
	if err != nil {
		return fmt.Errorf("error packing reply: %v", err)
	}

	lenHdr := struct{ Len uint16 }{Len: uint16(len(rbuf))}
	err = binary.Write(conn, binary.BigEndian, lenHdr)
	if err != nil {
		return err
	}

	_, err = conn.Write(rbuf)
	return err
}

func (m *miniDNS) handle(msg *dnsmessage.Message) *dnsmessage.Message {
	reply := &dnsmessage.Message{
		Header: dnsmessage.Header{
			ID:       msg.ID,
			Response: true,
			RCode:    dnsmessage.RCodeSuccess,

			// We're authoritative for the zones we're serving.
			// We should either set this, or RecursionAvailable, otherwise
			// some client libraries will complain.
			Authoritative: true,
		},
		Questions: msg.Questions,
	}

	q := msg.Questions[0]
	if answers, ok := m.answers[q.Name.String()]; ok {
		for _, ans := range answers {
			if q.Type == ans.Header.Type {
				log.Infof("-> %s %v", q.Type, ans.Body)
				reply.Answers = append(reply.Answers, ans)
			}
		}
	} else {
		log.Infof("-> NXERROR")
		reply.Header.RCode = dnsmessage.RCodeNameError
	}

	return reply
}

func (m *miniDNS) loadZones(f *os.File) {
	scanner := bufio.NewScanner(f)
	lineno := 0
	for scanner.Scan() {
		lineno++
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		vs := regexp.MustCompile(`\s+`).Split(line, 3)
		if len(vs) != 3 {
			log.Fatalf("line %d: invalid format", lineno)
		}
		domain, t, value := vs[0], vs[1], vs[2]
		if !strings.HasSuffix(domain, ".") {
			domain += "."
		}

		var body dnsmessage.ResourceBody
		var qType dnsmessage.Type
		switch strings.ToLower(t) {
		case "a":
			qType = dnsmessage.TypeA
			ip := net.ParseIP(value).To4()
			if ip == nil {
				log.Fatalf("line %d: invalid IP %q", lineno, value)
			}
			a := &dnsmessage.AResource{}
			copy(a.A[:], ip[:4])
			body = a
		case "aaaa":
			qType = dnsmessage.TypeAAAA
			ip := net.ParseIP(value).To16()
			if ip == nil {
				log.Fatalf("line %d: invalid IP %q", lineno, value)
			}
			aaaa := &dnsmessage.AAAAResource{}
			copy(aaaa.AAAA[:], ip[:16])
			body = aaaa
		case "mx":
			qType = dnsmessage.TypeMX
			if !strings.HasPrefix(value, ".") {
				value += "."
			}

			body = &dnsmessage.MXResource{
				Pref: 10,
				MX:   dnsmessage.MustNewName(value),
			}
		case "txt":
			qType = dnsmessage.TypeTXT

			// Cut value in chunks of 255 bytes.
			chunks := []string{}
			v := value
			for len(v) > 254 {
				chunks = append(chunks, v[:254])
				v = v[254:]
			}
			chunks = append(chunks, v)
			body = &dnsmessage.TXTResource{
				TXT: chunks,
			}
		default:
			log.Fatalf("line %d: unknown type %q", lineno, t)
		}

		answer := dnsmessage.Resource{
			Header: dnsmessage.ResourceHeader{
				Name:  dnsmessage.MustNewName(domain),
				Type:  qType,
				Class: dnsmessage.ClassINET,
			},
			Body: body,
		}
		m.answers[domain] = append(m.answers[domain], answer)
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("error reading zones: %v", err)
	}
}
