package git

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

func CurrentBranch(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

func Pull(dir, remote, branch string) ([]string, error) {
	before := head(dir)

	args := []string{"pull", "--ff-only", remote}
	if branch != "" {
		args = append(args, branch)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	out := strings.TrimSpace(string(output))

	if err != nil {
		switch {
		case strings.Contains(out, "does not appear to be a git repository"):
			return nil, fmt.Errorf("remote '%s' does not exist or is not a valid repository", remote)
		case strings.Contains(out, "Could not resolve host"):
			return nil, fmt.Errorf("cannot reach remote '%s' — host not found (check your URL or internet connection)", remote)
		case strings.Contains(out, "Connection refused"):
			return nil, fmt.Errorf("cannot reach remote '%s' — connection refused", remote)
		case strings.Contains(out, "Connection timed out"):
			return nil, fmt.Errorf("cannot reach remote '%s' — connection timed out", remote)
		case strings.Contains(out, "Authentication failed") || strings.Contains(out, "Permission denied"):
			return nil, fmt.Errorf("authentication failed for remote '%s' — check your credentials", remote)
		case strings.Contains(out, "couldn't find remote ref"):
			return nil, fmt.Errorf("branch '%s' does not exist on remote '%s'", branch, remote)
		default:
			return nil, fmt.Errorf("git pull failed: %w", err)
		}
	}

	after := head(dir)
	if after == "" || after == before {
		return nil, nil
	}

	commits, err := newCommits(dir, before, after)
	if err != nil || len(commits) == 0 {
		return []string{after[:7]}, nil
	}
	return commits, nil
}

func head(dir string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir

	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func newCommits(dir, from, to string) ([]string, error) {
	rev := to
	if from != "" {
		rev = from + ".." + to
	}

	cmd := exec.Command("git", "log", "--format=%h%x00%s%x00%an", rev)
	cmd.Dir = dir

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var commits []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		parts := strings.SplitN(line, "\x00", 3)
		if len(parts) != 3 {
			continue
		}
		subject := []rune(parts[1])
		if len(subject) > 72 {
			subject = append(subject[:69], []rune("...")...)
		}
		commits = append(commits, fmt.Sprintf("%s %s (%s)", parts[0], string(subject), parts[2]))
	}
	return commits, nil
}

func RunCommand(dir, command string, timeout time.Duration) error {
	log.Printf("running: %s", command)

	base := strings.Fields(command)
	if len(base) == 0 {
		return fmt.Errorf("empty command in config")
	}

	if _, err := exec.LookPath(base[0]); err != nil {
		return fmt.Errorf("command not found: '%s' — check your config or install it", base[0])
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 10 * time.Second

	output, err := cmd.CombinedOutput()
	out := strings.TrimSpace(string(output))

	if err != nil {
		lines := lastLines(out, 30)
		if lines != "" {
			log.Printf("command output (last 30 lines):\n%s", lines)
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("command timed out after %s and was killed: %s", timeout, command)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("command failed with exit code %d: %s", exitErr.ExitCode(), command)
		}
		return fmt.Errorf("command failed: %s — %w", command, err)
	}

	log.Printf("command finished successfully")
	return nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return "...\n" + strings.Join(lines[len(lines)-n:], "\n")
}
