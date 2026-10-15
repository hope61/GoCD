package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Repo struct {
	Dir      string   `yaml:"dir"`
	Remote   string   `yaml:"remote"`
	Branch   string   `yaml:"branch"`
	Commands []string `yaml:"commands"`
}

type Config struct {
	Repos []Repo `yaml:"repos"`
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

	for i := range cfg.Repos {
		if cfg.Repos[i].Remote == "" {
			cfg.Repos[i].Remote = "origin"
		}
		if cfg.Repos[i].Branch == "" {
			cfg.Repos[i].Branch = "main"
		}
	}

	return &cfg, nil
}
