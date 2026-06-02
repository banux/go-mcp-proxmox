package proxmox

import (
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	valid := map[string]string{
		EnvURL:         "https://pve.example.com:8006",
		EnvTokenID:     "mcp@pve!claude",
		EnvTokenSecret: "12345678-1234-1234-1234-123456789abc",
	}

	tests := []struct {
		name     string
		env      map[string]string
		wantErrs []string // substrings expected in the error; empty means success
		check    func(t *testing.T, cfg Config)
	}{
		{
			name: "valid config",
			env:  valid,
			check: func(t *testing.T, cfg Config) {
				if cfg.URL != valid[EnvURL] || cfg.TokenID != valid[EnvTokenID] || cfg.TokenSecret != valid[EnvTokenSecret] {
					t.Errorf("unexpected config: %+v", cfg)
				}
				if cfg.InsecureTLS {
					t.Error("InsecureTLS should default to false")
				}
			},
		},
		{
			name: "insecure TLS opt-in",
			env:  merge(valid, map[string]string{EnvInsecureTLS: "true"}),
			check: func(t *testing.T, cfg Config) {
				if !cfg.InsecureTLS {
					t.Error("InsecureTLS should be true")
				}
			},
		},
		{
			name:     "multiple missing vars reported together",
			env:      map[string]string{EnvTokenID: "mcp@pve!claude"},
			wantErrs: []string{EnvURL, EnvTokenSecret},
		},
		{
			name:     "invalid token ID format",
			env:      merge(valid, map[string]string{EnvTokenID: "mcp-claude"}),
			wantErrs: []string{"user@realm!tokenid"},
		},
		{
			name:     "invalid URL",
			env:      merge(valid, map[string]string{EnvURL: "not a url"}),
			wantErrs: []string{"not a valid URL"},
		},
		{
			name:     "invalid insecure TLS value",
			env:      merge(valid, map[string]string{EnvInsecureTLS: "maybe"}),
			wantErrs: []string{EnvInsecureTLS},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{EnvURL, EnvTokenID, EnvTokenSecret, EnvInsecureTLS} {
				t.Setenv(key, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			cfg, err := LoadConfig()
			if len(tt.wantErrs) == 0 {
				if err != nil {
					t.Fatalf("LoadConfig() error = %v, want nil", err)
				}
				tt.check(t, cfg)
				return
			}
			if err == nil {
				t.Fatalf("LoadConfig() = %+v, want error containing %v", cfg, tt.wantErrs)
			}
			for _, want := range tt.wantErrs {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestConfigStringRedactsSecret(t *testing.T) {
	cfg := Config{
		URL:         "https://pve.example.com:8006",
		TokenID:     "mcp@pve!claude",
		TokenSecret: "super-secret-uuid",
	}
	s := cfg.String()
	if strings.Contains(s, "super-secret-uuid") {
		t.Errorf("Config.String() leaks the token secret: %s", s)
	}
	if !strings.Contains(s, "[REDACTED]") {
		t.Errorf("Config.String() should mark the secret as redacted: %s", s)
	}
}

func merge(maps ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}
