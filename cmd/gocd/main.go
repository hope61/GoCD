package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/hope61/GoCD/internal/config"
	"github.com/hope61/GoCD/internal/git"
	"github.com/hope61/GoCD/internal/github"
	"github.com/hope61/GoCD/internal/logger"
	"github.com/hope61/GoCD/internal/webhook"
)

func main() {

	showLogs := flag.Bool("logs", false, "show the latest 50 lines of logs")
	flag.BoolVar(showLogs, "l", false, "show the latest 50 lines of logs")
	followLogs := flag.Bool("f", false, "follow logs in real time (use with -l)")
	edit := flag.Bool("e", false, "edit config.yaml (like crontab -e)")
	addHook := flag.Bool("add-hook", false, "add the webhook to the given repos: gocd --add-hook owner/repo ...")
	removeHook := flag.Bool("remove-hook", false, "remove the webhook from the given repos: gocd --remove-hook owner/repo ...")
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

	if *edit {
		editConfig(filepath.Join(binDir, "config.yaml"))
		return
	}

	cfg, err := config.Load(filepath.Join(binDir, "config.yaml"))
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	if *addHook || *removeHook {
		manageHooks(cfg, *addHook, flag.Args())
		return
	}

	if len(cfg.Repos) == 0 {
		log.Fatalf("no repos in config")
	}

	checks := make(chan struct{}, 1)
	trigger := func() {
		select {
		case checks <- struct{}{}:
		default:
		}
	}
	trigger()

	if cfg.Mode == "webhook" {
		go func() {
			log.Fatal(http.ListenAndServe(cfg.Listen, webhook.Handler(cfg.Secret, trigger)))
		}()
	} else {
		go func() {
			for {
				time.Sleep(cfg.Interval)
				trigger()
			}
		}()
	}

	for range checks {
		logFile := logger.Setup(binDir)
		checkRepos(cfg.Repos)
		logFile.Close()
	}
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
