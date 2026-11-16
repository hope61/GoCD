package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(write(t, "repos:\n  - dir: /srv/app\n"))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Mode != "poll" || cfg.Listen != ":9000" || cfg.Interval != 5*time.Minute || cfg.Timeout != 30*time.Minute {
		t.Fatalf("wrong defaults: %+v", cfg)
	}
	repo := cfg.Repos[0]
	if repo.Remote != "origin" || repo.Mode != "poll" || repo.Branch != "" || len(repo.Commands) != 0 {
		t.Fatalf("wrong repo defaults: %+v", repo)
	}
}

func TestLoadKeepsValues(t *testing.T) {
	cfg, err := Load(write(t, `mode: webhook
interval: 30s
timeout: 1h
listen: 127.0.0.1:9100
secret: s3cret
url: https://hook.example.com/
repos:
  - dir: /srv/a
    remote: upstream
    branch: develop
    mode: poll
    commands:
      - make build
      - make restart
  - dir: /srv/b
`))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Mode != "webhook" || cfg.Interval != 30*time.Second || cfg.Timeout != time.Hour ||
		cfg.Listen != "127.0.0.1:9100" || cfg.Secret != "s3cret" || cfg.URL != "https://hook.example.com/" {
		t.Fatalf("values not kept: %+v", cfg)
	}

	a, b := cfg.Repos[0], cfg.Repos[1]
	if a.Remote != "upstream" || a.Branch != "develop" || a.Mode != "poll" ||
		len(a.Commands) != 2 || a.Commands[0] != "make build" || a.Commands[1] != "make restart" {
		t.Fatalf("repo a not kept: %+v", a)
	}
	if b.Mode != "webhook" || b.Remote != "origin" {
		t.Fatalf("repo b should inherit webhook mode and default remote: %+v", b)
	}
}

func TestLoadNonPositiveDurationsUseDefaults(t *testing.T) {
	cfg, err := Load(write(t, "interval: 0s\ntimeout: -1m\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Interval != 5*time.Minute || cfg.Timeout != 30*time.Minute {
		t.Fatalf("expected defaults, got interval %s timeout %s", cfg.Interval, cfg.Timeout)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"wrong mode", "mode: banana\n", "mode must be poll or webhook"},
		{"wrong repo mode", "repos:\n  - dir: /srv/a\n    mode: banana\n", "/srv/a: mode must be poll or webhook"},
		{"webhook without secret", "mode: webhook\nrepos:\n  - dir: /srv/a\n", "webhook mode needs a secret"},
		{"repo webhook without secret", "repos:\n  - dir: /srv/a\n    mode: webhook\n", "webhook mode needs a secret"},
		{"setting written twice", "mode: poll\nmode: webhook\n", "a setting is written twice"},
		{"broken yaml", "repos: [\n", ""},
		{"bad duration", "interval: soon\n", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(write(t, tt.content))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q in error, got %q", tt.want, err)
			}
		})
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if !os.IsNotExist(err) {
		t.Fatalf("expected not-exist error, got %v", err)
	}
}
