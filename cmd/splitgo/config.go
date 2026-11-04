package main

import (
	"cmp"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/mgnsk/splitgo/mail"
)

// Config is the application configuration.
type Config struct {
	ListenAddr  string
	DatabaseDir string
	BaseURL     string
	SMTPURL     string
	MailFrom    string
	AllowSignup bool
}

// LoadConfig loads the configuration from environment variables.
func LoadConfig() (*Config, error) {
	var errs []error

	c := &Config{
		ListenAddr:  cmp.Or(os.Getenv("LISTEN_ADDR"), ":8080"),
		DatabaseDir: os.Getenv("DATABASE_DIR"),
		BaseURL:     strings.TrimSuffix(os.Getenv("BASE_URL"), "/"),
		SMTPURL:     os.Getenv("SMTP_URL"),
		MailFrom:    os.Getenv("MAIL_FROM"),
	}

	if v := os.Getenv("ALLOW_SIGNUP"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, fmt.Errorf("allow_signup: %w", err))
		}
		c.AllowSignup = b
	}

	if c.DatabaseDir == "" {
		errs = append(errs, fmt.Errorf("database_dir: is required"))
	}

	if u, err := url.Parse(c.BaseURL); c.BaseURL == "" || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		errs = append(errs, fmt.Errorf("base_url: must be an absolute http(s) URL"))
	}

	if c.SMTPURL != "" {
		if c.MailFrom == "" {
			errs = append(errs, fmt.Errorf("mail_from: is required when smtp_url is set"))
		} else if _, err := mail.ParseFrom(c.MailFrom); err != nil {
			errs = append(errs, fmt.Errorf("mail_from: %w", err))
		}
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return c, nil
}

// SecureCookies reports whether cookies should be marked Secure.
func (c *Config) SecureCookies() bool {
	return strings.HasPrefix(c.BaseURL, "https://")
}
