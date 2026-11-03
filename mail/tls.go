package mail

import (
	"context"
	"crypto/tls"
	"net/smtp"
)

func sendImplicitTLS(ctx context.Context, s *SMTPSender, to string, body []byte) error {
	dialer := &tls.Dialer{Config: &tls.Config{ServerName: s.host}}
	conn, err := dialer.DialContext(ctx, "tcp", s.addr)
	if err != nil {
		return err
	}

	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return err
	}
	defer c.Close()

	if s.auth != nil {
		if err := c.Auth(s.auth); err != nil {
			return err
		}
	}

	if err := c.Mail(s.from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}

	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(body); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}

	return c.Quit()
}
