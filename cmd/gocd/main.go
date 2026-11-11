package main

import (
	"flag"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/hope61/GoCD/internal/config"
	"github.com/hope61/GoCD/internal/git"
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
	check := flag.Bool("check", false, "check all repos now and exit")
	flag.BoolVar(check, "c", false, "check all repos now and exit")
	flag.Usage = usage
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

	if *check {
		logFile := logger.Setup(binDir)
		checkRepos(cfg.Repos, cfg.Timeout)
		log.Printf("checked %d repos", len(cfg.Repos))
		logFile.Close()
		return
	}

	var pollRepos, hookRepos []config.Repo
	for _, repo := range cfg.Repos {
		if repo.Mode == "webhook" {
			hookRepos = append(hookRepos, repo)
		} else {
			pollRepos = append(pollRepos, repo)
		}
	}

	logFile := logger.Setup(binDir)
	if len(hookRepos) > 0 {
		log.Printf("started webhook on %s for %d repos", cfg.Listen, len(hookRepos))
	}
	if len(pollRepos) > 0 {
		log.Printf("started polling every %s for %d repos", cfg.Interval, len(pollRepos))
	}
	logFile.Close()

	pollChecks := make(chan struct{}, 1)
	hookChecks := make(chan struct{}, 1)
	pollChecks <- struct{}{}
	hookChecks <- struct{}{}

	if len(hookRepos) > 0 {
		go func() {
			log.Fatal(http.ListenAndServe(cfg.Listen, webhook.Handler(cfg.Secret, func() { signal(hookChecks) })))
		}()
	}
	if len(pollRepos) > 0 {
		go func() {
			for {
				time.Sleep(cfg.Interval)
				signal(pollChecks)
			}
		}()
	}

	go func() {
		for {
			time.Sleep(24 * time.Hour)
			signal(pollChecks)
			signal(hookChecks)
		}
	}()

	for {
		var repos []config.Repo
		select {
		case <-pollChecks:
			repos = pollRepos
		case <-hookChecks:
			repos = hookRepos
		}
		if len(repos) == 0 {
			continue
		}

		logFile := logger.Setup(binDir)
		checkRepos(repos, cfg.Timeout)
		logFile.Close()
	}
}

func signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

func checkRepos(repos []config.Repo, timeout time.Duration) {
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
			err = git.RunCommand(repo.Dir, cmd, timeout)
			if err != nil {
				log.Printf("ERROR: %v", err)
				break
			}
		}
	}
}
