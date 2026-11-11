package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/hope61/GoCD/internal/config"
	"github.com/hope61/GoCD/internal/github"
	"github.com/hope61/GoCD/internal/logger"
)

func usage() {
	fmt.Fprintf(flag.CommandLine.Output(), `gocd keeps the git repos in config.yaml up to date.
Run without flags it starts the service loop.

Config file: %s

Repos:
  -c, --check                   check all repos now and exit

Logs:
  -l, --logs                    show the last 50 log lines
  -l -f                         follow the logs live

Config:
  -e                            edit config.yaml safely (like crontab -e)

Webhooks:
  --add-hook owner/repo ...     add the webhook to repos
  --remove-hook owner/repo ...  remove the webhook from repos

  -h, --help                    show this help
`, filepath.Join(logger.BinaryDir(), "config.yaml"))
}

func manageHooks(cfg *config.Config, add bool, repos []string) {
	if len(repos) == 0 {
		log.Fatalf("no repos given, e.g. gocd --add-hook owner/repo")
	}
	if cfg.URL == "" {
		log.Fatalf("set url in config.yaml to the public webhook url")
	}
	if add && cfg.Secret == "" {
		log.Fatalf("set secret in config.yaml first (make secret)")
	}

	failed := false
	for _, repo := range repos {
		var err error
		if add {
			err = github.AddHook(repo, cfg.URL, cfg.Secret)
		} else {
			err = github.RemoveHook(repo, cfg.URL)
		}

		if err != nil {
			log.Printf("ERROR: %s — %v", repo, err)
			failed = true
			continue
		}
		log.Printf("done: %s", repo)
	}

	if failed {
		os.Exit(1)
	}
}
