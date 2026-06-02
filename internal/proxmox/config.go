// Package proxmox provides a minimal client for the Proxmox VE REST API,
// authenticated exclusively with an API token.
package proxmox

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
)

// Environment variables read by LoadConfig.
const (
	EnvURL         = "PROXMOX_URL"
	EnvTokenID     = "PROXMOX_TOKEN_ID"
	EnvTokenSecret = "PROXMOX_TOKEN_SECRET"
	EnvInsecureTLS = "PROXMOX_INSECURE_TLS"
	EnvAllowWrite  = "PROXMOX_ALLOW_WRITE"
)

// tokenIDRe matches the Proxmox API token ID format: user@realm!tokenid.
var tokenIDRe = regexp.MustCompile(`^[^@!\s]+@[^@!\s]+![^@!\s]+$`)

// Config holds the connection settings for a Proxmox VE cluster.
type Config struct {
	// URL is the base URL of the Proxmox API, e.g. https://pve.example.com:8006.
	URL string
	// TokenID is the API token identifier, in the form user@realm!tokenid.
	TokenID string
	// TokenSecret is the API token secret. Never log this value.
	TokenSecret string
	// InsecureTLS disables TLS certificate verification when true.
	InsecureTLS bool
	// AllowWrite enables the mutating tools. When false (the default), the
	// server exposes only read-only tools.
	AllowWrite bool
}

// String implements fmt.Stringer and redacts the token secret.
func (c Config) String() string {
	secret := ""
	if c.TokenSecret != "" {
		secret = "[REDACTED]"
	}
	return fmt.Sprintf("proxmox.Config{URL: %q, TokenID: %q, TokenSecret: %q, InsecureTLS: %t, AllowWrite: %t}",
		c.URL, c.TokenID, secret, c.InsecureTLS, c.AllowWrite)
}

// LoadConfig builds a Config from environment variables. It validates every
// variable and returns a single error aggregating all missing or invalid ones.
func LoadConfig() (Config, error) {
	var cfg Config
	var errs []error

	rawURL := os.Getenv(EnvURL)
	switch {
	case rawURL == "":
		errs = append(errs, fmt.Errorf("%s is required (e.g. https://pve.example.com:8006)", EnvURL))
	default:
		u, err := url.Parse(rawURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			errs = append(errs, fmt.Errorf("%s is not a valid URL: %q", EnvURL, rawURL))
		} else {
			cfg.URL = rawURL
		}
	}

	tokenID := os.Getenv(EnvTokenID)
	switch {
	case tokenID == "":
		errs = append(errs, fmt.Errorf("%s is required (format: user@realm!tokenid)", EnvTokenID))
	case !tokenIDRe.MatchString(tokenID):
		errs = append(errs, fmt.Errorf("%s must match the format user@realm!tokenid, got %q", EnvTokenID, tokenID))
	default:
		cfg.TokenID = tokenID
	}

	if secret := os.Getenv(EnvTokenSecret); secret == "" {
		errs = append(errs, fmt.Errorf("%s is required", EnvTokenSecret))
	} else {
		cfg.TokenSecret = secret
	}

	if raw := os.Getenv(EnvInsecureTLS); raw != "" {
		insecure, err := strconv.ParseBool(raw)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s must be a boolean (true/false), got %q", EnvInsecureTLS, raw))
		} else {
			cfg.InsecureTLS = insecure
		}
	}

	if raw := os.Getenv(EnvAllowWrite); raw != "" {
		allow, err := strconv.ParseBool(raw)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s must be a boolean (true/false), got %q", EnvAllowWrite, raw))
		} else {
			cfg.AllowWrite = allow
		}
	}

	if len(errs) > 0 {
		return Config{}, fmt.Errorf("invalid Proxmox configuration:\n%w", errors.Join(errs...))
	}
	return cfg, nil
}
