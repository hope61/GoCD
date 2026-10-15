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

	configPath := filepath.Join(binDir, "config.yaml")
	cfg, err := config.Load(configPath)

	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatalf("failed to load config: %v", err)
	}

	if cfg != nil && len(cfg.Repos) > 0 {
		hasError := false
		hasChanges := false

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

			changed, err := git.Pull(repo.Dir, repo.Remote, repo.Branch)
			if err != nil {
				log.Printf("ERROR: %s — %v", repo.Dir, err)
				hasError = true
				continue
			}

			if !changed {
				continue
			}

			hasChanges = true
			log.Printf("pulled new changes in %s", repo.Dir)

			for _, cmd := range repo.Commands {
				err = git.RunCommand(repo.Dir, cmd)
				if err != nil {
					log.Printf("ERROR: %v", err)
					hasError = true
					break
				}
			}
		}

		if !hasChanges && !hasError {
			log.Printf("all repos up to date")
		}

		if hasError {
			os.Exit(1)
		}
	} else {
		repoDir := *dir
		if repoDir == "" {
			repoDir = binDir
		}

		_, err := git.Pull(repoDir, *remote, *branch)
		if err != nil {
			log.Printf("ERROR: %v", err)
			os.Exit(1)
		}
	}
}
