package tests

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

const secret = "bench-secret"
const trials = 10

var bin string
var results bytes.Buffer

func TestMain(m *testing.M) {
	if os.Getenv("GOCD_BENCH") == "" {
		fmt.Println("skipping: run with GOCD_BENCH=1 (make bench)")
		os.Exit(0)
	}

	tmp, err := os.MkdirTemp("", "gocd-bench")
	if err != nil {
		panic(err)
	}
	bin = filepath.Join(tmp, "gocd")
	if out, err := exec.Command("go", "build", "-o", bin, "../cmd/gocd").CombinedOutput(); err != nil {
		panic(string(out))
	}

	fmt.Fprintf(&results, "# Benchmark results\n\n%s, %s\n", time.Now().Format("2006-01-02 15:04"), machine())

	code := m.Run()

	os.WriteFile("results.md", results.Bytes(), 0644)
	os.RemoveAll(tmp)
	os.Exit(code)
}

type env struct {
	dir, work, pulls, gitLog string
	port                     int
	cmd                      *exec.Cmd
}

func start(t *testing.T, mode string, interval time.Duration) *env {
	t.Helper()
	dir := t.TempDir()
	e := &env{
		dir:    dir,
		work:   filepath.Join(dir, "work"),
		pulls:  filepath.Join(dir, "pulls.txt"),
		gitLog: filepath.Join(dir, "git-calls.txt"),
		port:   freePort(t),
	}

	run(t, dir, "git", "init", "-q", "--bare", "origin.git")
	run(t, dir, "git", "clone", "-q", "origin.git", "work")
	commit(t, e.work)
	run(t, e.work, "git", "push", "-q", "origin", "HEAD")
	run(t, dir, "git", "clone", "-q", "origin.git", "deploy")

	realGit, _ := exec.LookPath("git")
	os.Mkdir(filepath.Join(dir, "bin"), 0755)
	wrapper := fmt.Sprintf("#!/bin/sh\necho \"$1\" >> %s\nexec %s \"$@\"\n", e.gitLog, realGit)
	os.WriteFile(filepath.Join(dir, "bin", "git"), []byte(wrapper), 0755)

	data, _ := os.ReadFile(bin)
	os.WriteFile(filepath.Join(dir, "gocd"), data, 0755)

	cfg := fmt.Sprintf("mode: %s\ninterval: %s\nlisten: 127.0.0.1:%d\nsecret: %s\nrepos:\n  - dir: %s\n    commands:\n      - echo pulled >> %s\n",
		mode, interval, e.port, secret, filepath.Join(dir, "deploy"), e.pulls)
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0644)

	logFile, _ := os.Create(filepath.Join(dir, "out.log"))
	e.cmd = exec.Command(filepath.Join(dir, "gocd"))
	e.cmd.Env = append(os.Environ(), "PATH="+filepath.Join(dir, "bin")+":"+os.Getenv("PATH"))
	e.cmd.Stdout, e.cmd.Stderr = logFile, logFile
	if err := e.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		e.cmd.Process.Kill()
		e.cmd.Wait()
		logFile.Close()
	})

	waitFor(t, func() bool { return e.gitCalls("pull") >= 1 })
	if mode == "webhook" {
		waitFor(t, func() bool {
			r, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", e.port))
			if err == nil {
				r.Body.Close()
			}
			return err == nil
		})
	}
	time.Sleep(200 * time.Millisecond)
	return e
}

func (e *env) push(t *testing.T) time.Time {
	commit(t, e.work)
	run(t, e.work, "git", "push", "-q", "origin", "HEAD")
	return time.Now()
}

func (e *env) sendWebhook(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main"}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)

	req, _ := http.NewRequest("POST", fmt.Sprintf("http://127.0.0.1:%d/", e.port), bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("webhook returned %d", resp.StatusCode)
	}
}

func (e *env) waitPulls(t *testing.T, n int) time.Time {
	waitFor(t, func() bool { return lines(e.pulls) >= n })
	return time.Now()
}

func (e *env) gitCalls(sub string) int {
	data, _ := os.ReadFile(e.gitLog)
	return strings.Count(string(data), sub+"\n")
}

func (e *env) rssKB() int {
	out, _ := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(e.cmd.Process.Pid)).Output()
	n, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return n
}

func TestLatency(t *testing.T) {
	fmt.Fprintf(&results, "\n## Latency: push → post-pull command ran (%d trials each)\n\n", trials)
	fmt.Fprintf(&results, "| Mode | Min | Median | Mean | Max |\n|---|---|---|---|---|\n")

	for _, interval := range []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second} {
		e := start(t, "poll", interval)
		var lat []time.Duration
		for i := 1; i <= trials; i++ {
			time.Sleep(time.Duration(rand.Int63n(int64(interval))))
			pushed := e.push(t)
			lat = append(lat, e.waitPulls(t, i).Sub(pushed))
		}
		row(fmt.Sprintf("poll, interval %s", interval), lat)
	}

	e := start(t, "webhook", time.Minute)
	var lat []time.Duration
	for i := 1; i <= trials; i++ {
		time.Sleep(time.Duration(rand.Int63n(int64(time.Second))))
		pushed := e.push(t)
		e.sendWebhook(t)
		lat = append(lat, e.waitPulls(t, i).Sub(pushed))
	}
	row("webhook", lat)
}

func TestIdleCost(t *testing.T) {
	const window = 30 * time.Second
	fmt.Fprintf(&results, "\n## Idle cost: %s with no pushes, one repo\n\n", window)
	fmt.Fprintf(&results, "| Mode | git pulls | Pulls per day (extrapolated) | Memory (RSS) |\n|---|---|---|---|\n")

	for _, c := range []struct {
		mode     string
		interval time.Duration
	}{{"poll", 2 * time.Second}, {"poll", 5 * time.Second}, {"webhook", time.Minute}} {
		e := start(t, c.mode, c.interval)
		before := e.gitCalls("pull")
		time.Sleep(window)
		n := e.gitCalls("pull") - before

		name, perDay := "webhook", "0"
		if c.mode == "poll" {
			name = fmt.Sprintf("poll, interval %s", c.interval)
			perDay = strconv.Itoa(int(24 * time.Hour / c.interval))
		}
		fmt.Fprintf(&results, "| %s | %d | %s | %.1f MB |\n", name, n, perDay, float64(e.rssKB())/1024)
	}
}

func TestCheckCost(t *testing.T) {
	fmt.Fprintf(&results, "\n## Cost of one check with nothing new (%d runs)\n\n", trials)
	fmt.Fprintf(&results, "| Remote | Min | Median | Mean | Max |\n|---|---|---|---|---|\n")

	e := start(t, "webhook", time.Minute)
	deploy := filepath.Join(e.dir, "deploy")
	var local []time.Duration
	for i := 0; i < trials; i++ {
		s := time.Now()
		run(t, deploy, "git", "pull", "--ff-only", "-q", "origin")
		local = append(local, time.Since(s))
	}
	row("local bare repo", local)

	var remote []time.Duration
	for i := 0; i < trials; i++ {
		s := time.Now()
		if err := exec.Command("git", "ls-remote", "https://github.com/golang/go.git", "HEAD").Run(); err != nil {
			fmt.Fprintf(&results, "| github.com | no network: %v | | | |\n", err)
			return
		}
		remote = append(remote, time.Since(s))
	}
	row("github.com over https (ls-remote)", remote)
}

func row(name string, d []time.Duration) {
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	var sum time.Duration
	for _, x := range d {
		sum += x
	}
	f := func(x time.Duration) string { return x.Round(time.Millisecond).String() }
	fmt.Fprintf(&results, "| %s | %s | %s | %s | %s |\n", name, f(d[0]), f(d[len(d)/2]), f(sum/time.Duration(len(d))), f(d[len(d)-1]))
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

func commit(t *testing.T, dir string) {
	run(t, dir, "git", "-c", "user.name=bench", "-c", "user.email=bench@local", "commit", "-q", "--allow-empty", "-m", "bench")
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Minute)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func lines(path string) int {
	data, _ := os.ReadFile(path)
	return bytes.Count(data, []byte("\n"))
}

func freePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func machine() string {
	out, _ := exec.Command("uname", "-sm").Output()
	return strings.TrimSpace(string(out))
}
