// Package mail sends email.
package mail

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"net/url"
	"strings"
	"time"
)

// Message is an email message.
type Message struct {
	To      string
	Subject string
	Body    string
}

// Sender sends email messages.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// LogSender logs messages instead of sending them. Useful for development.
type LogSender struct{}

// Send logs the message.
func (LogSender) Send(_ context.Context, msg Message) error {
	slog.Info("email (not sent, SMTP not configured)",
		slog.String("to", msg.To),
		slog.String("subject", msg.Subject),
		slog.String("body", msg.Body),
	)
	return nil
}

// SMTPSender sends email over SMTP.
type SMTPSender struct {
	addr     string
	host     string
	auth     smtp.Auth
	from     string
	implicit bool // implicit TLS (smtps)
}

// NewSMTPSender creates an SMTP sender from a URL like
// smtp://user:pass@host:587 (STARTTLS) or smtps://user:pass@host:465.
func NewSMTPSender(rawURL, from string) (*SMTPSender, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid SMTP URL: %w", err)
	}

	s := &SMTPSender{from: from, host: u.Hostname()}

	switch u.Scheme {
	case "smtp":
		s.addr = net.JoinHostPort(u.Hostname(), cmpOr(u.Port(), "587"))
	case "smtps":
		s.addr = net.JoinHostPort(u.Hostname(), cmpOr(u.Port(), "465"))
		s.implicit = true
	default:
		return nil, fmt.Errorf("invalid SMTP URL scheme %q", u.Scheme)
	}

	if u.User != nil {
		pass, _ := u.User.Password()
		s.auth = smtp.PlainAuth("", u.User.Username(), pass, s.host)
	}

	return s, nil
}

// Send sends the message.
func (s *SMTPSender) Send(ctx context.Context, msg Message) error {
	body := buildMessage(s.from, msg)

	if !s.implicit {
		return smtp.SendMail(s.addr, s.auth, s.from, []string{msg.To}, body)
	}

	return sendImplicitTLS(ctx, s, msg.To, body)
}

func buildMessage(from string, msg Message) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", msg.To)
	fmt.Fprintf(&b, "Subject: %s\r\n", msg.Subject)
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(strings.ReplaceAll(msg.Body, "\n", "\r\n"))
	return []byte(b.String())
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
