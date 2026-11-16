package git

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/hope61/GoCD/internal/testutil"
)

func TestMain(m *testing.M) {
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

func TestCurrentBranch(t *testing.T) {
	r := testutil.NewRepos(t)

	branch, err := CurrentBranch(r.Srv)
	if err != nil || branch != "main" {
		t.Fatalf("got %q %v, want main", branch, err)
	}

	testutil.Git(t, r.Srv, "checkout", "-q", "-b", "develop")
	if branch, _ := CurrentBranch(r.Srv); branch != "develop" {
		t.Fatalf("got %q, want develop", branch)
	}

	if _, err := CurrentBranch(t.TempDir()); err == nil || !strings.Contains(err.Error(), "failed to get current branch") {
		t.Fatalf("expected an error outside a repo, got %v", err)
	}
}

func TestPullNoChanges(t *testing.T) {
	r := testutil.NewRepos(t)

	commits, err := Pull(r.Srv, "origin", "")
	if err != nil || len(commits) != 0 {
		t.Fatalf("got %v %v, want no commits", commits, err)
	}
}

func TestPullNewCommits(t *testing.T) {
	r := testutil.NewRepos(t)
	r.Push(t, "first", "second", "third")

	commits, err := Pull(r.Srv, "origin", "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 3 {
		t.Fatalf("expected 3 commits, got %v", commits)
	}
	for i, want := range []string{"third", "second", "first"} {
		fields := strings.Fields(commits[i])
		if len(fields[0]) < 7 || !strings.HasSuffix(commits[i], " "+want+" (tester)") {
			t.Fatalf("commit %d: got %q, want a short hash then %q (tester)", i, commits[i], want)
		}
	}
	if r.Head(t) != testutil.Git(t, r.Src, "rev-parse", "HEAD") {
		t.Fatal("server copy did not move to the new commit")
	}

	commits, err = Pull(r.Srv, "origin", "")
	if err != nil || len(commits) != 0 {
		t.Fatalf("second pull should find nothing, got %v %v", commits, err)
	}
}

func TestPullCutsLongSubjects(t *testing.T) {
	r := testutil.NewRepos(t)
	r.Push(t, strings.Repeat("x", 100), strings.Repeat("ж", 100), strings.Repeat("y", 72))

	commits, err := Pull(r.Srv, "origin", "")
	if err != nil || len(commits) != 3 {
		t.Fatalf("got %v %v", commits, err)
	}

	if !strings.Contains(commits[0], " "+strings.Repeat("y", 72)+" (tester)") {
		t.Fatalf("72 characters should not be cut: %q", commits[0])
	}
	if !strings.Contains(commits[1], " "+strings.Repeat("ж", 69)+"... (tester)") || !utf8.ValidString(commits[1]) {
		t.Fatalf("cyrillic subject cut wrong: %q", commits[1])
	}
	if !strings.Contains(commits[2], " "+strings.Repeat("x", 69)+"... (tester)") {
		t.Fatalf("long subject cut wrong: %q", commits[2])
	}
}

func TestPullIntoEmptyClone(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	src := filepath.Join(root, "src")
	srv := filepath.Join(root, "srv")
	testutil.Git(t, root, "init", "-q", "--bare", "-b", "main", remote)
	testutil.Git(t, root, "clone", "-q", remote, srv)
	testutil.Git(t, root, "clone", "-q", remote, src)
	testutil.Git(t, src, "checkout", "-q", "-B", "main")
	testutil.Git(t, src, "commit", "-q", "--allow-empty", "-m", "one")
	testutil.Git(t, src, "commit", "-q", "--allow-empty", "-m", "two")
	testutil.Git(t, src, "push", "-q", "origin", "main")

	commits, err := Pull(srv, "origin", "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 || !strings.HasSuffix(commits[0], " two (tester)") {
		t.Fatalf("expected both commits, got %v", commits)
	}
}

func TestPullErrors(t *testing.T) {
	t.Run("unknown remote", func(t *testing.T) {
		r := testutil.NewRepos(t)
		_, err := Pull(r.Srv, "nosuch", "")
		if err == nil || !strings.Contains(err.Error(), "remote 'nosuch' does not exist") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("missing branch", func(t *testing.T) {
		r := testutil.NewRepos(t)
		_, err := Pull(r.Srv, "origin", "nope")
		if err == nil || !strings.Contains(err.Error(), "branch 'nope' does not exist on remote 'origin'") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("diverged history is never merged", func(t *testing.T) {
		r := testutil.NewRepos(t)
		testutil.Git(t, r.Srv, "commit", "-q", "--allow-empty", "-m", "local change")
		local := r.Head(t)
		r.Push(t, "remote change")

		commits, err := Pull(r.Srv, "origin", "")
		if err == nil || !strings.Contains(err.Error(), "git pull failed") || commits != nil {
			t.Fatalf("got %v %v, want a pull error", commits, err)
		}
		if r.Head(t) != local {
			t.Fatal("server copy changed even though the pull failed")
		}
	})
}

func TestRunCommand(t *testing.T) {
	t.Run("runs in the repo folder", func(t *testing.T) {
		dir := t.TempDir()
		if err := RunCommand(dir, "echo hi > out.txt", time.Minute); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(dir, "out.txt"))
		if err != nil || strings.TrimSpace(string(data)) != "hi" {
			t.Fatalf("command did not run in %s: %q %v", dir, data, err)
		}
	})

	tests := []struct {
		name    string
		command string
		want    string
	}{
		{"exit code", "sh -c 'exit 3'", "command failed with exit code 3"},
		{"missing program", "definitely-not-a-real-command-xyz --flag", "command not found: 'definitely-not-a-real-command-xyz'"},
		{"empty command", "   ", "empty command in config"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RunCommand(t.TempDir(), tt.command, time.Minute)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want %q", err, tt.want)
			}
		})
	}
}

func TestRunCommandTimeoutKillsChildren(t *testing.T) {
	dir := t.TempDir()
	start := time.Now()

	err := RunCommand(dir, "sleep 60 & echo $! > child.pid; wait", 500*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "command timed out after 500ms and was killed") {
		t.Fatalf("got %v, want a timeout error", err)
	}
	if took := time.Since(start); took > 15*time.Second {
		t.Fatalf("timeout took %s", took)
	}

	data, err := os.ReadFile(filepath.Join(dir, "child.pid"))
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))

	deadline := time.Now().Add(3 * time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			t.Fatalf("child process %d is still running after the timeout", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestLastLines(t *testing.T) {
	if got := lastLines("a\nb", 30); got != "a\nb" {
		t.Fatalf("short output changed: %q", got)
	}

	var lines []string
	for i := 1; i <= 40; i++ {
		lines = append(lines, strconv.Itoa(i))
	}
	got := lastLines(strings.Join(lines, "\n"), 30)
	if !strings.HasPrefix(got, "...\n11\n") || !strings.HasSuffix(got, "\n40") || strings.Count(got, "\n") != 30 {
		t.Fatalf("wrong last lines: %q", got)
	}
}
