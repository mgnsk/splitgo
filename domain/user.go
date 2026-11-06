// Package domain contains the core domain types.
package domain

import (
	"strings"
	"sync"
	"time"

	"github.com/mgnsk/webkit/snowflake"
)

// User is a user, identified by email address.
type User struct {
	ID       snowflake.ID
	Email    string
	Name     string
	Currency string
	Theme    Theme
	// Timezone is the IANA time zone of the user's browser, or empty.
	Timezone string
}

var locations sync.Map

// Location returns the user's time zone, or the server's if unknown.
func (u *User) Location() *time.Location {
	if u.Timezone == "" {
		return time.Local
	}
	if loc, ok := locations.Load(u.Timezone); ok {
		return loc.(*time.Location)
	}
	loc, err := time.LoadLocation(u.Timezone)
	if err != nil {
		return time.Local
	}
	locations.Store(u.Timezone, loc)
	return loc
}

// ValidTimezone reports whether tz is a known IANA time zone name.
func ValidTimezone(tz string) bool {
	if tz == "" || tz == "Local" || len(tz) > 64 {
		return false
	}
	_, err := time.LoadLocation(tz)
	return err == nil
}

// Theme is a color theme preference.
type Theme string

// Themes.
const (
	ThemeSystem Theme = "system"
	ThemeLight  Theme = "light"
	ThemeDark   Theme = "dark"
)

// Valid reports whether t is a known theme.
func (t Theme) Valid() bool {
	return t == ThemeSystem || t == ThemeLight || t == ThemeDark
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
