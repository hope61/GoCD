package main

import (
	"flag"
	"log"
	"path/filepath"
	"time"

	"github.com/hope61/GoCD/internal/config"
	"github.com/hope61/GoCD/internal/git"
	"github.com/hope61/GoCD/internal/logger"
)

func main() {
	showLogs := flag.Bool("logs", false, "show the latest 50 lines of logs")
	flag.BoolVar(showLogs, "l", false, "show the latest 50 lines of logs")
	followLogs := flag.Bool("f", false, "follow logs in real time (use with -l)")
	flag.Parse()

	binDir := logger.BinaryDir()

	if *showLogs {
		if *followLogs {
			logger.FollowLogs(binDir, 50)
		} else {
			logger.ShowLogs(binDir, 50)
		}
		return
	}

	cfg, err := config.Load(filepath.Join(binDir, "config.yaml"))
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	if len(cfg.Repos) == 0 {
		log.Fatalf("no repos in config")
	}

	for {
		// reopened every pass so the log file rolls over to the new day
		logFile := logger.Setup(binDir)
		checkRepos(cfg.Repos)
		logFile.Close()

		time.Sleep(cfg.Interval)
	}
}

func checkRepos(repos []config.Repo) {
	for _, repo := range repos {
		currentBranch, err := git.CurrentBranch(repo.Dir)
		if err != nil {
			log.Printf("ERROR: %s — %v", repo.Dir, err)
			continue
		}

		if repo.Branch != "" && repo.Branch != currentBranch {
			log.Printf("WARNING: %s is on '%s' but config expects '%s', skipping", repo.Dir, currentBranch, repo.Branch)
			continue
		}

		changed, err := git.Pull(repo.Dir, repo.Remote, repo.Branch)
		if err != nil {
			log.Printf("ERROR: %s — %v", repo.Dir, err)
			continue
		}

		if !changed {
			continue
		}

		log.Printf("pulled new changes in %s", repo.Dir)

		for _, cmd := range repo.Commands {
			err = git.RunCommand(repo.Dir, cmd)
			if err != nil {
				log.Printf("ERROR: %v", err)
				break
			}
		}
	}
}
