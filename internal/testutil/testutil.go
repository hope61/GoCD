package testutil

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type Repos struct {
	Remote string
	Src    string
	Srv    string
}

func Git(t testing.TB, dir string, args ...string) string {
	t.Helper()
	base := []string{"-c", "user.name=tester", "-c", "user.email=tester@example.com", "-c", "commit.gpgsign=false", "-c", "init.defaultBranch=main"}
	cmd := exec.Command("git", append(base, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func NewRepos(t testing.TB) Repos {
	t.Helper()
	root := t.TempDir()
	r := Repos{
		Remote: filepath.Join(root, "remote.git"),
		Src:    filepath.Join(root, "src"),
		Srv:    filepath.Join(root, "srv"),
	}
	Git(t, root, "init", "-q", "--bare", "-b", "main", r.Remote)
	Git(t, root, "clone", "-q", r.Remote, r.Src)
	Git(t, r.Src, "checkout", "-q", "-B", "main")
	Git(t, r.Src, "commit", "-q", "--allow-empty", "-m", "init")
	Git(t, r.Src, "push", "-q", "origin", "main")
	Git(t, root, "clone", "-q", r.Remote, r.Srv)
	return r
}

func (r Repos) Push(t testing.TB, messages ...string) {
	t.Helper()
	for _, msg := range messages {
		Git(t, r.Src, "commit", "-q", "--allow-empty", "-m", msg)
	}
	Git(t, r.Src, "push", "-q", "origin", "main")
}

func (r Repos) Head(t testing.TB) string {
	t.Helper()
	return Git(t, r.Srv, "rev-parse", "HEAD")
}
