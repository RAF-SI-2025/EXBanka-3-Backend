package service

import (
	"bufio"
	"net"
	"strings"
	"testing"
)

// fakeSMTP accepts one connection and speaks just enough of SMTP for
// smtp.SendMail to complete, then returns the address it's listening on.
func fakeSMTP(t *testing.T) (host string, port int, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveSMTP(conn)
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", addr.Port, func() { _ = ln.Close() }
}

func serveSMTP(conn net.Conn) {
	defer conn.Close()
	w := bufio.NewWriter(conn)
	r := bufio.NewReader(conn)
	reply := func(s string) { _, _ = w.WriteString(s + "\r\n"); _ = w.Flush() }

	reply("220 fake ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			reply("250 ok")
		case strings.HasPrefix(cmd, "MAIL"), strings.HasPrefix(cmd, "RCPT"):
			reply("250 ok")
		case cmd == "DATA":
			reply("354 end with .")
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimSpace(l) == "." {
					break
				}
			}
			reply("250 ok queued")
		case cmd == "QUIT":
			reply("221 bye")
			return
		default:
			reply("250 ok")
		}
	}
}

func TestSMTPEmailService_Send_Success(t *testing.T) {
	host, port, stop := fakeSMTP(t)
	defer stop()

	svc := NewSMTPEmailService(host, port, "noreply@bank.com")
	if err := svc.Send("user@example.com", "Hi", "Body text"); err != nil {
		t.Fatalf("Send: %v", err)
	}
}

func TestSMTPEmailService_Send_DialError(t *testing.T) {
	// Port 1 is not listening — SendMail fails to dial.
	svc := NewSMTPEmailService("127.0.0.1", 1, "noreply@bank.com")
	if err := svc.Send("user@example.com", "Hi", "Body"); err == nil {
		t.Error("expected an error dialing a dead SMTP port")
	}
}
