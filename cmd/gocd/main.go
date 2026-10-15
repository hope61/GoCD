package main

import (
	"errors"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/hope61/GoCD/internal/config"
	"github.com/hope61/GoCD/internal/git"
	"github.com/hope61/GoCD/internal/logger"
)

func main() {
	dir := flag.String("dir", "", "the directory of the repo (used when no config file exists)")
	remote := flag.String("remote", "origin", "remote to pull from")
	branch := flag.String("branch", "", "branch to pull (defaults to current branch)")
	flag.Parse()

	binDir := logger.BinaryDir()
	logFile := logger.Setup(binDir)
	defer logFile.Close()

	log.Printf("--- run started ---")

	configPath := filepath.Join(binDir, "config.yaml")
	cfg, err := config.Load(configPath)

	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatalf("failed to load config: %v", err)
	}

	if cfg != nil && len(cfg.Repos) > 0 {
		log.Printf("loaded %d repo(s) from config", len(cfg.Repos))
		hasError := false

		for _, repo := range cfg.Repos {
			currentBranch, err := git.CurrentBranch(repo.Dir)
			if err != nil {
				log.Printf("ERROR: %s — %v", repo.Dir, err)
				hasError = true
				continue
			}

			if repo.Branch != "" && repo.Branch != currentBranch {
				log.Printf("WARNING: %s is on '%s' but config expects '%s', skipping", repo.Dir, currentBranch, repo.Branch)
				continue
			}

			branch := repo.Branch
			if branch == "" {
				branch = currentBranch
			}

			log.Printf("pulling %s (%s/%s)", repo.Dir, repo.Remote, branch)

			changed, err := git.Pull(repo.Dir, repo.Remote, repo.Branch)
			if err != nil {
				log.Printf("ERROR: %v", err)
				hasError = true
				continue
			}

			if changed && len(repo.Commands) > 0 {
				log.Printf("new changes detected, running %d command(s)", len(repo.Commands))
				for _, cmd := range repo.Commands {
					err = git.RunCommand(repo.Dir, cmd)
					if err != nil {
						log.Printf("ERROR: %v", err)
						hasError = true
						break
					}
				}
			}
		}

		if hasError {
			os.Exit(1)
		}
	} else {
		repoDir := *dir
		if repoDir == "" {
			repoDir = binDir
		}

		log.Printf("no config found, using flags — repo: %s | remote: %s | branch: %s", repoDir, *remote, *branch)

		_, err := git.Pull(repoDir, *remote, *branch)
		if err != nil {
			log.Printf("ERROR: %v", err)
			os.Exit(1)
		}
	}
}
