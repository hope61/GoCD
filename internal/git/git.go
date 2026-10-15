package git

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
)

func Pull(dir, remote, branch string) (bool, error) {
	cmd := exec.Command("git", "pull", "--ff-only", remote, branch)
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		log.Printf("git output:\n%s", strings.TrimSpace(string(output)))
	}

	if err != nil {
		out := string(output)

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

	if strings.Contains(string(output), "Already up to date.") {
		log.Printf("repo is already up to date")
		return false, nil
	}

	log.Printf("git pull completed successfully")
	return true, nil
}

func RunCommand(dir, command string) error {
	log.Printf("running: %s", command)

	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		log.Printf("command output:\n%s", strings.TrimSpace(string(output)))
	}

	if err != nil {
		return fmt.Errorf("command failed: %s — %w", command, err)
	}

	log.Printf("command finished successfully")
	return nil
}
