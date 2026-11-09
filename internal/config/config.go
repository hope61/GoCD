package config

import (
	"errors"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Repo struct {
	Dir      string   `yaml:"dir"`
	Remote   string   `yaml:"remote"`
	Branch   string   `yaml:"branch"`
	Commands []string `yaml:"commands"`
}

type Config struct {
	Repos    []Repo        `yaml:"repos"`
	Mode     string        `yaml:"mode"`
	Interval time.Duration `yaml:"interval"`
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
		return nil, err
	}

	switch cfg.Mode {
	case "":
		cfg.Mode = "poll"
	case "poll":
	case "webhook":
		if cfg.Secret == "" {
			return nil, errors.New("webhook mode needs a secret")
		}
	default:
		return nil, errors.New("mode must be poll or webhook")
	}

	if cfg.Listen == "" {
		cfg.Listen = ":9000"
	}

	if cfg.Interval <= 0 {
		cfg.Interval = 5 * time.Minute
	}

	for i := range cfg.Repos {
		if cfg.Repos[i].Remote == "" {
			cfg.Repos[i].Remote = "origin"
		}
	}

	return &cfg, nil
}
