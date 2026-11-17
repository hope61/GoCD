package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hope61/GoCD/internal/config"
	"github.com/hope61/GoCD/internal/history"
	"github.com/hope61/GoCD/internal/testutil"
)

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

func repo(dir string, commands ...string) config.Repo {
	return config.Repo{Dir: dir, Remote: "origin", Mode: "poll", Commands: commands}
}

func TestSignalNeverBlocks(t *testing.T) {
	ch := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 10; i++ {
			signal(ch)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("signal blocked on a full channel")
	}
	if len(ch) != 1 {
		t.Fatalf("10 signals should become 1, got %d", len(ch))
	}
}

func TestUsage(t *testing.T) {
	var buf bytes.Buffer
	flag.CommandLine.SetOutput(&buf)
	t.Cleanup(func() { flag.CommandLine.SetOutput(nil) })

	usage()

	for _, want := range []string{"Config file: ", "config.yaml", "-c, --check", "-l, --logs", "-l -f", "--history [n]", "-e", "--add-hook owner/repo", "--remove-hook owner/repo", "-h, --help"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("help is missing %q:\n%s", want, buf.String())
		}
	}
}

func TestCheckReposRunsCommandsInOrder(t *testing.T) {
	r := testutil.NewRepos(t)
	r.Push(t, "fix navbar", "update list")
	logs := captureLog(t)

	checkRepos(t.TempDir(), []config.Repo{repo(r.Srv, "echo one >> order.txt", "echo two >> order.txt")}, time.Minute)

	data, _ := os.ReadFile(filepath.Join(r.Srv, "order.txt"))
	if string(data) != "one\ntwo\n" {
		t.Fatalf("commands did not run in order: %q", data)
	}
	out := logs.String()
	for _, want := range []string{"pulled 2 new commits in " + r.Srv, " update list (tester)", " fix navbar (tester)", "running: echo one >> order.txt", "command finished successfully"} {
		if !strings.Contains(out, want) {
			t.Fatalf("log is missing %q:\n%s", want, out)
		}
	}
}

func TestCheckReposSingleCommitWording(t *testing.T) {
	r := testutil.NewRepos(t)
	r.Push(t, "only one")
	logs := captureLog(t)

	checkRepos(t.TempDir(), []config.Repo{repo(r.Srv)}, time.Minute)

	if !strings.Contains(logs.String(), "pulled 1 new commit in "+r.Srv) {
		t.Fatalf("got:\n%s", logs)
	}
}

func TestCheckReposShowsFiveCommitsMax(t *testing.T) {
	r := testutil.NewRepos(t)
	r.Push(t, "c1", "c2", "c3", "c4", "c5", "c6", "c7")
	logs := captureLog(t)

	checkRepos(t.TempDir(), []config.Repo{repo(r.Srv)}, time.Minute)

	out := logs.String()
	if !strings.Contains(out, "pulled 7 new commits") || !strings.Contains(out, " c7 (tester)") ||
		!strings.Contains(out, " c3 (tester)") || strings.Contains(out, " c2 (tester)") || !strings.Contains(out, "... and 2 more") {
		t.Fatalf("expected 5 newest commits and '... and 2 more', got:\n%s", out)
	}
}

func TestCheckReposNoChangesRunsNothing(t *testing.T) {
	r := testutil.NewRepos(t)
	logs := captureLog(t)

	checkRepos(t.TempDir(), []config.Repo{repo(r.Srv, "touch ran.txt")}, time.Minute)

	if _, err := os.Stat(filepath.Join(r.Srv, "ran.txt")); err == nil {
		t.Fatal("commands ran without new code")
	}
	if logs.Len() != 0 {
		t.Fatalf("nothing should be logged, got:\n%s", logs)
	}
}

func TestCheckReposStopsAfterFailedCommand(t *testing.T) {
	r := testutil.NewRepos(t)
	r.Push(t, "change")
	logs := captureLog(t)

	checkRepos(t.TempDir(), []config.Repo{repo(r.Srv, "false", "touch after.txt")}, time.Minute)

	if _, err := os.Stat(filepath.Join(r.Srv, "after.txt")); err == nil {
		t.Fatal("a command ran after a failed one")
	}
	if !strings.Contains(logs.String(), "ERROR: command failed with exit code 1: false") {
		t.Fatalf("got:\n%s", logs)
	}
}

func TestCheckReposTimeout(t *testing.T) {
	r := testutil.NewRepos(t)
	r.Push(t, "change")
	logs := captureLog(t)

	start := time.Now()
	checkRepos(t.TempDir(), []config.Repo{repo(r.Srv, "sleep 30", "touch after.txt")}, 300*time.Millisecond)

	if time.Since(start) > 15*time.Second {
		t.Fatal("timeout did not stop the command")
	}
	if !strings.Contains(logs.String(), "ERROR: command timed out after 300ms and was killed: sleep 30") {
		t.Fatalf("got:\n%s", logs)
	}
	if _, err := os.Stat(filepath.Join(r.Srv, "after.txt")); err == nil {
		t.Fatal("a command ran after a timed out one")
	}
}

func TestCheckReposSkipsWrongBranch(t *testing.T) {
	r := testutil.NewRepos(t)
	before := r.Head(t)
	r.Push(t, "change")
	logs := captureLog(t)

	rp := repo(r.Srv, "touch ran.txt")
	rp.Branch = "develop"
	checkRepos(t.TempDir(), []config.Repo{rp}, time.Minute)

	if !strings.Contains(logs.String(), "WARNING: "+r.Srv+" is on 'main' but config expects 'develop', skipping") {
		t.Fatalf("got:\n%s", logs)
	}
	if r.Head(t) != before {
		t.Fatal("repo on the wrong branch was pulled")
	}
}

func TestCheckReposContinuesAfterBrokenRepo(t *testing.T) {
	r := testutil.NewRepos(t)
	r.Push(t, "change")
	notRepo := t.TempDir()
	logs := captureLog(t)

	checkRepos(t.TempDir(), []config.Repo{repo(notRepo), repo(r.Srv, "touch ran.txt")}, time.Minute)

	if !strings.Contains(logs.String(), "ERROR: "+notRepo+" — failed to get current branch") {
		t.Fatalf("got:\n%s", logs)
	}
	if _, err := os.Stat(filepath.Join(r.Srv, "ran.txt")); err != nil {
		t.Fatal("the second repo was not deployed after the first one failed")
	}
}

func TestCheckReposPullError(t *testing.T) {
	r := testutil.NewRepos(t)
	logs := captureLog(t)

	rp := repo(r.Srv, "touch ran.txt")
	rp.Remote = "nosuch"
	checkRepos(t.TempDir(), []config.Repo{rp}, time.Minute)

	if !strings.Contains(logs.String(), "ERROR: "+r.Srv+" — remote 'nosuch' does not exist") {
		t.Fatalf("got:\n%s", logs)
	}
}

func readHistory(t *testing.T, dir string) []history.Entry {
	t.Helper()
	data, _ := os.ReadFile(filepath.Join(dir, "history.jsonl"))
	var entries []history.Entry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var e history.Entry
		if json.Unmarshal([]byte(line), &e) == nil {
			entries = append(entries, e)
		}
	}
	return entries
}

func TestCheckReposSavesHistory(t *testing.T) {
	r := testutil.NewRepos(t)
	dir := t.TempDir()
	captureLog(t)

	r.Push(t, "fix navbar", "update list")
	checkRepos(dir, []config.Repo{repo(r.Srv, "echo ok")}, time.Minute)

	r.Push(t, "break build")
	checkRepos(dir, []config.Repo{repo(r.Srv, "false", "echo never")}, time.Minute)

	checkRepos(dir, []config.Repo{repo(r.Srv, "echo ok")}, time.Minute)

	entries := readHistory(t, dir)
	if len(entries) != 2 {
		t.Fatalf("expected 2 deploys (no entry without new commits), got %+v", entries)
	}

	ok := entries[0]
	if ok.Repo != r.Srv || len(ok.Commit) < 7 || ok.Message != "update list (tester)" || ok.Commits != 2 || ok.Error != "" || ok.Time.IsZero() || ok.Took <= 0 {
		t.Fatalf("wrong ok entry: %+v", ok)
	}

	failed := entries[1]
	if failed.Message != "break build (tester)" || failed.Commits != 1 || failed.Error != "command failed with exit code 1: false" {
		t.Fatalf("wrong failed entry: %+v", failed)
	}
}

func TestCheckReposHistoryErrorDoesNotStopDeploy(t *testing.T) {
	r := testutil.NewRepos(t)
	r.Push(t, "change")
	logs := captureLog(t)

	checkRepos(filepath.Join(t.TempDir(), "missing"), []config.Repo{repo(r.Srv, "touch ran.txt")}, time.Minute)

	if _, err := os.Stat(filepath.Join(r.Srv, "ran.txt")); err != nil {
		t.Fatal("the deploy did not run")
	}
	if !strings.Contains(logs.String(), "WARNING: failed to save history") {
		t.Fatalf("got:\n%s", logs)
	}
}

func TestAsk(t *testing.T) {
	in := bufio.NewReader(strings.NewReader("  YeS \n"))
	if got := ask(in, ""); got != "yes" {
		t.Fatalf("got %q", got)
	}
}

func TestPickEditorFromEnv(t *testing.T) {
	t.Setenv("VISUAL", "code --wait")
	t.Setenv("EDITOR", "vim")
	if e, err := pickEditor(nil); err != nil || e != "code --wait" {
		t.Fatalf("VISUAL should win, got %q %v", e, err)
	}

	t.Setenv("VISUAL", "")
	if e, err := pickEditor(nil); err != nil || e != "vim" {
		t.Fatalf("EDITOR should be used, got %q %v", e, err)
	}
}

func TestPickEditorAsksAndSaves(t *testing.T) {
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "nano"), []byte("#!/bin/sh\n"), 0755)
	home := t.TempDir()
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	t.Setenv("PATH", bin)
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")

	e, err := pickEditor(bufio.NewReader(strings.NewReader("abc\n9\n1\n")))
	if err != nil || e != "nano" {
		t.Fatalf("got %q %v", e, err)
	}
	data, _ := os.ReadFile(filepath.Join(home, ".zshrc"))
	if !strings.Contains(string(data), "export EDITOR=nano") {
		t.Fatalf("editor not saved to .zshrc: %q", data)
	}
}

func TestPickEditorNoneInstalled(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	t.Setenv("PATH", t.TempDir())

	if _, err := pickEditor(nil); err == nil || !strings.Contains(err.Error(), "no editor found") {
		t.Fatalf("got %v", err)
	}
}

func TestSaveEditor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	t.Setenv("SHELL", "/bin/bash")
	saveEditor("vim")
	saveEditor("nano")
	data, _ := os.ReadFile(filepath.Join(home, ".bashrc"))
	if strings.Count(string(data), "export EDITOR=") != 1 || !strings.Contains(string(data), "export EDITOR=vim") {
		t.Fatalf("expected one EDITOR line for vim, got %q", data)
	}

	t.Setenv("SHELL", "/usr/bin/fish")
	saveEditor("vim")
	if entries, _ := os.ReadDir(home); len(entries) != 1 {
		t.Fatalf("unknown shell should not write any file, got %d files", len(entries))
	}
}
