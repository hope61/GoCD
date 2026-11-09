package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type hook struct {
	ID     int `json:"id"`
	Config struct {
		URL string `json:"url"`
	} `json:"config"`
}

func AddHook(repo, url, secret string) error {
	ids, err := hookIDs(repo, url)
	if err != nil {
		return err
	}
	if len(ids) > 0 {
		return fmt.Errorf("%s already has a webhook for %s", repo, url)
	}

	body, _ := json.Marshal(map[string]any{
		"name":   "web",
		"active": true,
		"events": []string{"push"},
		"config": map[string]string{
			"url":          url,
			"content_type": "json",
			"secret":       secret,
		},
	})

	_, err = gh(body, "api", "-X", "POST", "repos/"+repo+"/hooks", "--input", "-")
	return err
}

func RemoveHook(repo, url string) error {
	ids, err := hookIDs(repo, url)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return fmt.Errorf("%s has no webhook for %s", repo, url)
	}

	for _, id := range ids {
		if _, err := gh(nil, "api", "-X", "DELETE", fmt.Sprintf("repos/%s/hooks/%d", repo, id)); err != nil {
			return err
		}
	}
	return nil
}

func hookIDs(repo, url string) ([]int, error) {
	out, err := gh(nil, "api", "--paginate", "--slurp", "repos/"+repo+"/hooks")
	if err != nil {
		return nil, err
	}

	var pages [][]hook
	if err := json.Unmarshal(out, &pages); err != nil {
		return nil, fmt.Errorf("unexpected response from github: %w", err)
	}

	var ids []int
	for _, page := range pages {
		for _, h := range page {
			if h.Config.URL == url {
				ids = append(ids, h.ID)
			}
		}
	}
	return ids, nil
}

func gh(stdin []byte, args ...string) ([]byte, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, fmt.Errorf("gh CLI not found — install it and run 'gh auth login'")
	}

	cmd := exec.Command("gh", args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("github: %s", strings.TrimSpace(stderr.String()))
	}
	return out, nil
}
