package config

import (
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
	Interval time.Duration `yaml:"interval"`
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
