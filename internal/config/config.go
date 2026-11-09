package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Repo struct {
	Dir      string   `yaml:"dir"`
	Remote   string   `yaml:"remote"`
	Branch   string   `yaml:"branch"`
	Mode     string   `yaml:"mode"`
	Commands []string `yaml:"commands"`
}

type Config struct {
	Repos    []Repo        `yaml:"repos"`
	Mode     string        `yaml:"mode"`
	Interval time.Duration `yaml:"interval"`
	Timeout  time.Duration `yaml:"timeout"`
	Listen   string        `yaml:"listen"`
	Secret   string        `yaml:"secret"`
	URL      string        `yaml:"url"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		if strings.Contains(err.Error(), "already defined") {
			return nil, fmt.Errorf("%w\na setting is written twice in config.yaml, delete one of them (e.g. both 'mode: poll' and 'mode: webhook')", err)
		}
		return nil, err
	}

	if cfg.Mode == "" {
		cfg.Mode = "poll"
	}
	if cfg.Mode != "poll" && cfg.Mode != "webhook" {
		return nil, errors.New("mode must be poll or webhook")
	}

	if cfg.Listen == "" {
		cfg.Listen = ":9000"
	}

	if cfg.Interval <= 0 {
		cfg.Interval = 5 * time.Minute
	}

	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Minute
	}

	for i := range cfg.Repos {
		repo := &cfg.Repos[i]
		if repo.Remote == "" {
			repo.Remote = "origin"
		}
		if repo.Mode == "" {
			repo.Mode = cfg.Mode
		}
		if repo.Mode != "poll" && repo.Mode != "webhook" {
			return nil, fmt.Errorf("%s: mode must be poll or webhook", repo.Dir)
		}
		if repo.Mode == "webhook" && cfg.Secret == "" {
			return nil, errors.New("webhook mode needs a secret")
		}
	}

	return &cfg, nil
}
