package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hope61/GoCD/internal/testutil"
)

var builtBinary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "gocd-e2e-*")
	if err != nil {
		panic(err)
	}
	builtBinary = filepath.Join(dir, "gocd")
	if out, err := exec.Command("go", "build", "-o", builtBinary, ".").CombinedOutput(); err != nil {
		panic(fmt.Sprintf("build failed: %v\n%s", err, out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func install(t *testing.T, cfg string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "gocd")
	data, err := os.ReadFile(builtBinary)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(bin, data, 0755)
	if cfg != "" {
		os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0644)
	}
	return dir, bin
}

func gocd(t *testing.T, bin, stdin string, args ...string) (string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = strings.NewReader(stdin)
	out, _ := cmd.CombinedOutput()
	return string(out), cmd.ProcessState.ExitCode()
}

func start(t *testing.T, bin string, args ...string) *syncBuffer {
	t.Helper()
	out := &syncBuffer{}
	cmd := exec.Command(bin, args...)
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	return out
}

func waitFor(t *testing.T, what string, timeout time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func lines(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return strings.Count(string(data), "\n")
}

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

func post(t *testing.T, addr, event, secret string, body []byte) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", event)
	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode
}

func waitForPort(t *testing.T, addr string) {
	t.Helper()
	waitFor(t, "the webhook port", 10*time.Second, func() bool {
		c, err := net.Dial("tcp", addr)
		if err == nil {
			c.Close()
		}
		return err == nil
	})
}

func TestE2EHelp(t *testing.T) {
	dir, bin := install(t, "")
	out, code := gocd(t, bin, "", "-h")

	real, _ := filepath.EvalSymlinks(dir)
	if code != 0 || !strings.Contains(out, "Config file: "+filepath.Join(real, "config.yaml")) || !strings.Contains(out, "-c, --check") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
}

func TestE2EConfigErrors(t *testing.T) {
	tests := []struct {
		name string
		cfg  string
		want string
	}{
		{"missing config", "", "failed to load config"},
		{"wrong mode", "mode: banana\n", "failed to load config: mode must be poll or webhook"},
		{"webhook without secret", "mode: webhook\nrepos:\n  - dir: /tmp\n", "webhook mode needs a secret"},
		{"no repos", "mode: poll\n", "no repos in config"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, bin := install(t, tt.cfg)
			out, code := gocd(t, bin, "", "-c")
			if code != 1 || !strings.Contains(out, tt.want) {
				t.Fatalf("exit %d, want 1 and %q:\n%s", code, tt.want, out)
			}
		})
	}
}

func TestE2ECheckAndLogs(t *testing.T) {
	r := testutil.NewRepos(t)
	r.Push(t, "fix navbar", "update list")
	dir, bin := install(t, fmt.Sprintf("repos:\n  - dir: %s\n    commands:\n      - echo deployed\n", r.Srv))

	out, _ := gocd(t, bin, "", "-l")
	if !strings.Contains(out, "no log files found") {
		t.Fatalf("-l before any log:\n%s", out)
	}

	out, code := gocd(t, bin, "", "-c")
	for _, want := range []string{"pulled 2 new commits in " + r.Srv, " update list (tester)", "running: echo deployed", "command finished successfully", "checked 1 repos"} {
		if code != 0 || !strings.Contains(out, want) {
			t.Fatalf("-c missing %q (exit %d):\n%s", want, code, out)
		}
	}

	files, _ := filepath.Glob(filepath.Join(dir, "logs", "gocd_*.log"))
	if len(files) != 1 {
		t.Fatalf("expected one daily log file, got %v", files)
	}

	out, _ = gocd(t, bin, "", "--check")
	if strings.Contains(out, "pulled") || !strings.Contains(out, "checked 1 repos") {
		t.Fatalf("second check should find nothing:\n%s", out)
	}

	out, _ = gocd(t, bin, "", "--logs")
	if !strings.Contains(out, "pulled 2 new commits") || strings.Count(out, "checked 1 repos") != 2 {
		t.Fatalf("-l should show both runs:\n%s", out)
	}
}

func TestE2EFollowLogsWaitsForFirstLog(t *testing.T) {
	dir, bin := install(t, "")
	out := start(t, bin, "-l", "-f")

	time.Sleep(700 * time.Millisecond)
	os.MkdirAll(filepath.Join(dir, "logs"), 0755)
	os.WriteFile(filepath.Join(dir, "logs", "gocd_2026-11-15.log"), []byte("first line\n"), 0644)

	waitFor(t, "the first log line", 5*time.Second, func() bool { return strings.Contains(out.String(), "first line") })

	f, _ := os.OpenFile(filepath.Join(dir, "logs", "gocd_2026-11-15.log"), os.O_APPEND|os.O_WRONLY, 0644)
	f.WriteString("second line\n")
	f.Close()
	waitFor(t, "a line added later", 5*time.Second, func() bool { return strings.Contains(out.String(), "second line") })
}

func TestE2EPollMode(t *testing.T) {
	r := testutil.NewRepos(t)
	_, bin := install(t, fmt.Sprintf("mode: poll\ninterval: 1s\nrepos:\n  - dir: %s\n    commands:\n      - echo pulled >> marker\n", r.Srv))
	marker := filepath.Join(r.Srv, "marker")

	out := start(t, bin)
	waitFor(t, "the startup line", 10*time.Second, func() bool { return strings.Contains(out.String(), "started polling every 1s for 1 repos") })

	r.Push(t, "first")
	waitFor(t, "the first poll deploy", 10*time.Second, func() bool { return lines(marker) == 1 })

	r.Push(t, "second")
	waitFor(t, "the second poll deploy", 10*time.Second, func() bool { return lines(marker) == 2 })

	if !strings.Contains(out.String(), " second (tester)") {
		t.Fatalf("commit info missing from service output:\n%s", out)
	}
}

func TestE2EWebhookMode(t *testing.T) {
	r := testutil.NewRepos(t)
	addr := freePort(t)
	_, bin := install(t, fmt.Sprintf("mode: webhook\nlisten: %s\nsecret: s3cret\nrepos:\n  - dir: %s\n    commands:\n      - echo pulled >> marker\n", addr, r.Srv))
	marker := filepath.Join(r.Srv, "marker")

	r.Push(t, "pushed while gocd was off")
	out := start(t, bin)
	waitFor(t, "the startup check", 10*time.Second, func() bool { return lines(marker) == 1 })
	if !strings.Contains(out.String(), "started webhook on "+addr+" for 1 repos") {
		t.Fatalf("startup line missing:\n%s", out)
	}
	waitForPort(t, addr)

	body := []byte(`{"ref":"refs/heads/main"}`)
	r.Push(t, "change")
	if code := post(t, addr, "push", "wrong", body); code != http.StatusUnauthorized {
		t.Fatalf("wrong secret got %d", code)
	}
	if code := post(t, addr, "push", "", body); code != http.StatusUnauthorized {
		t.Fatalf("missing signature got %d", code)
	}
	if code := post(t, addr, "ping", "s3cret", body); code != http.StatusNoContent {
		t.Fatalf("ping got %d", code)
	}
	resp, err := http.Get("http://" + addr + "/")
	if err != nil || resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET should be refused: %v %v", resp, err)
	}
	resp.Body.Close()

	time.Sleep(time.Second)
	if lines(marker) != 1 {
		t.Fatal("a rejected request or a ping started a deploy")
	}

	if code := post(t, addr, "push", "s3cret", body); code != http.StatusNoContent {
		t.Fatalf("signed push got %d", code)
	}
	waitFor(t, "the webhook deploy", 10*time.Second, func() bool { return lines(marker) == 2 })
}

func TestE2EMixedModesKeepTheirOwnRepos(t *testing.T) {
	poll := testutil.NewRepos(t)
	hook := testutil.NewRepos(t)
	addr := freePort(t)
	_, bin := install(t, fmt.Sprintf(`mode: webhook
interval: 1h
listen: %s
secret: s3cret
repos:
  - dir: %s
    commands:
      - echo pulled >> marker
  - dir: %s
    mode: poll
    commands:
      - echo pulled >> marker
`, addr, hook.Srv, poll.Srv))

	out := start(t, bin)
	waitFor(t, "both startup lines", 10*time.Second, func() bool {
		s := out.String()
		return strings.Contains(s, "started webhook on") && strings.Contains(s, "started polling every 1h0m0s for 1 repos")
	})
	waitForPort(t, addr)

	hook.Push(t, "hook change")
	poll.Push(t, "poll change")
	if code := post(t, addr, "push", "s3cret", []byte(`{}`)); code != http.StatusNoContent {
		t.Fatalf("signed push got %d", code)
	}

	waitFor(t, "the webhook repo deploy", 10*time.Second, func() bool { return lines(filepath.Join(hook.Srv, "marker")) == 1 })
	time.Sleep(time.Second)
	if lines(filepath.Join(poll.Srv, "marker")) != 0 {
		t.Fatal("a webhook deployed a poll repo")
	}
}

const fakeGh = `#!/bin/sh
echo "$@" >> "$GH_TEST_DIR/calls"
case "$*" in
  *"--paginate repos/hope61/old/hooks"*) /bin/cat "$GH_TEST_DIR/hooks.json" ;;
  *--paginate*) echo "[]" ;;
  *"-X POST"*) /bin/cat > /dev/null ;;
esac
`

func TestE2EHooks(t *testing.T) {
	ghDir := t.TempDir()
	os.WriteFile(filepath.Join(ghDir, "gh"), []byte(fakeGh), 0755)
	os.WriteFile(filepath.Join(ghDir, "hooks.json"), []byte(`[{"id":5,"config":{"url":"https://hook.example.com/"}}]`), 0644)
	t.Setenv("GH_TEST_DIR", ghDir)
	t.Setenv("PATH", ghDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, bin := install(t, "mode: poll\nsecret: s3cret\nurl: https://hook.example.com/\n")

	out, code := gocd(t, bin, "", "--add-hook", "hope61/new", "hope61/old")
	if code != 1 || !strings.Contains(out, "done: hope61/new") || !strings.Contains(out, "ERROR: hope61/old") || !strings.Contains(out, "already has a webhook") {
		t.Fatalf("add-hook: exit %d\n%s", code, out)
	}

	out, code = gocd(t, bin, "", "--remove-hook", "hope61/old")
	calls, _ := os.ReadFile(filepath.Join(ghDir, "calls"))
	if code != 0 || !strings.Contains(out, "done: hope61/old") || !strings.Contains(string(calls), "-X DELETE repos/hope61/old/hooks/5") {
		t.Fatalf("remove-hook: exit %d\n%s\ncalls:\n%s", code, out, calls)
	}

	if out, code := gocd(t, bin, "", "--add-hook"); code != 1 || !strings.Contains(out, "no repos given") {
		t.Fatalf("no repos: exit %d\n%s", code, out)
	}

	_, noURL := install(t, "mode: poll\nsecret: s3cret\n")
	if out, code := gocd(t, noURL, "", "--add-hook", "hope61/app"); code != 1 || !strings.Contains(out, "set url in config.yaml") {
		t.Fatalf("no url: exit %d\n%s", code, out)
	}

	_, noSecret := install(t, "mode: poll\nurl: https://hook.example.com/\n")
	if out, code := gocd(t, noSecret, "", "--add-hook", "hope61/app"); code != 1 || !strings.Contains(out, "set secret in config.yaml first") {
		t.Fatalf("no secret: exit %d\n%s", code, out)
	}
}

func TestE2EEditConfig(t *testing.T) {
	tools := t.TempDir()
	good := filepath.Join(tools, "good.yaml")
	bad := filepath.Join(tools, "bad.yaml")
	os.WriteFile(good, []byte("mode: poll\ninterval: 1m\nrepos:\n  - dir: /srv/app\n"), 0644)
	os.WriteFile(bad, []byte("mode: banana\n"), 0644)

	editor := func(name, script string) string {
		path := filepath.Join(tools, name)
		os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0755)
		return path
	}
	t.Setenv("VISUAL", "")

	t.Run("valid edit is saved", func(t *testing.T) {
		dir, bin := install(t, "")
		t.Setenv("EDITOR", editor("write-good", `/bin/cp "`+good+`" "$1"`))
		out, code := gocd(t, bin, "", "-e")
		saved, _ := os.ReadFile(filepath.Join(dir, "config.yaml"))
		want, _ := os.ReadFile(good)
		if code != 0 || !strings.Contains(out, "saved ") || !bytes.Equal(saved, want) {
			t.Fatalf("exit %d\n%s\nsaved: %q", code, out, saved)
		}
	})

	t.Run("no change", func(t *testing.T) {
		_, bin := install(t, "mode: poll\n")
		t.Setenv("EDITOR", editor("noop", "true\n"))
		if out, code := gocd(t, bin, "", "-e"); code != 0 || !strings.Contains(out, "no changes made") {
			t.Fatalf("exit %d\n%s", code, out)
		}
	})

	t.Run("broken config is never saved", func(t *testing.T) {
		dir, bin := install(t, "mode: poll\n")
		t.Setenv("EDITOR", editor("write-bad", `/bin/cp "`+bad+`" "$1"`))
		out, code := gocd(t, bin, "n\n", "-e")
		saved, _ := os.ReadFile(filepath.Join(dir, "config.yaml"))
		if code != 0 || !strings.Contains(out, "config has errors") || !strings.Contains(out, "changes discarded") || string(saved) != "mode: poll\n" {
			t.Fatalf("exit %d\n%s\nconfig now: %q", code, out, saved)
		}
	})

	t.Run("fix after an error", func(t *testing.T) {
		dir, bin := install(t, "mode: poll\n")
		count := filepath.Join(tools, "count")
		os.Remove(count)
		t.Setenv("EDITOR", editor("bad-then-good", `if [ -f "`+count+`" ]; then /bin/cp "`+good+`" "$1"; else : > "`+count+`"; /bin/cp "`+bad+`" "$1"; fi`+"\n"))
		out, code := gocd(t, bin, "y\n", "-e")
		saved, _ := os.ReadFile(filepath.Join(dir, "config.yaml"))
		want, _ := os.ReadFile(good)
		if code != 0 || !strings.Contains(out, "config has errors") || !strings.Contains(out, "saved ") || !bytes.Equal(saved, want) {
			t.Fatalf("exit %d\n%s\nsaved: %q", code, out, saved)
		}
	})
}
