// Package domain contains the core domain types.
package domain

import (
	"strings"

	"github.com/mgnsk/splitgo/pkg/snowflake"
)

// User is a user, identified by email address.
type User struct {
	ID       snowflake.ID
	Email    string
	Name     string
	Currency string
}

// DisplayName returns the name or, if empty, the local part of the email.
func (u *User) DisplayName() string {
	if u == nil {
		return "Unknown"
	}
	if name := strings.TrimSpace(u.Name); name != "" {
		return name
	}
	local, _, _ := strings.Cut(u.Email, "@")
	return local
}

// NormalizeEmail lowercases and trims an email address.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
