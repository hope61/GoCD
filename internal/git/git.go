package git

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
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

func Pull(dir, remote, branch string) (bool, error) {
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
			return false, fmt.Errorf("remote '%s' does not exist or is not a valid repository", remote)
		case strings.Contains(out, "Could not resolve host"):
			return false, fmt.Errorf("cannot reach remote '%s' — host not found (check your URL or internet connection)", remote)
		case strings.Contains(out, "Connection refused"):
			return false, fmt.Errorf("cannot reach remote '%s' — connection refused", remote)
		case strings.Contains(out, "Connection timed out"):
			return false, fmt.Errorf("cannot reach remote '%s' — connection timed out", remote)
		case strings.Contains(out, "Authentication failed") || strings.Contains(out, "Permission denied"):
			return false, fmt.Errorf("authentication failed for remote '%s' — check your credentials", remote)
		case strings.Contains(out, "couldn't find remote ref"):
			return false, fmt.Errorf("branch '%s' does not exist on remote '%s'", branch, remote)
		default:
			return false, fmt.Errorf("git pull failed: %w", err)
		}
	}

	if strings.Contains(out, "Already up to date.") {
		return false, nil
	}

	log.Printf("pulled new changes")
	return true, nil
}

func RunCommand(dir, command string) error {
	log.Printf("running: %s", command)

	base := strings.Fields(command)
	if len(base) == 0 {
		return fmt.Errorf("empty command in config")
	}

	if _, err := exec.LookPath(base[0]); err != nil {
		return fmt.Errorf("command not found: '%s' — check your config or install it", base[0])
	}

	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	out := strings.TrimSpace(string(output))

	if err != nil {
		lines := lastLines(out, 30)
		if lines != "" {
			log.Printf("command output (last 30 lines):\n%s", lines)
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
