package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const keep = 500

type Entry struct {
	Time    time.Time     `json:"time"`
	Repo    string        `json:"repo"`
	Commit  string        `json:"commit"`
	Message string        `json:"message"`
	Commits int           `json:"commits"`
	Error   string        `json:"error,omitempty"`
	Took    time.Duration `json:"took"`
}

func Add(dir string, e Entry) error {
	path := filepath.Join(dir, "history.jsonl")

	line, err := json.Marshal(e)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) <= keep {
		return nil
	}

	// write to a temp file and rename so a crash never leaves a half written history
	tmp := path + ".tmp"
	err = os.WriteFile(tmp, []byte(strings.Join(lines[len(lines)-keep:], "\n")+"\n"), 0644)
	if err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func Show(dir string, n int) {
	data, err := os.ReadFile(filepath.Join(dir, "history.jsonl"))
	if err != nil && !os.IsNotExist(err) {
		fmt.Printf("failed to read history: %v\n", err)
		return
	}

	var entries []Entry
	for _, line := range strings.Split(string(data), "\n") {
		var e Entry
		if json.Unmarshal([]byte(line), &e) == nil {
			entries = append(entries, e)
		}
	}

	if len(entries) == 0 {
		fmt.Println("no deploys yet")
		return
	}

	entries = entries[max(0, len(entries)-n):]
	slices.Reverse(entries)

	stat, err := os.Stdout.Stat()
	color := err == nil && stat.Mode()&os.ModeCharDevice != 0 && os.Getenv("NO_COLOR") == ""
	paint := func(code, s string) string {
		if !color {
			return s
		}
		return "\x1b[" + code + "m" + s + "\x1b[0m"
	}

	var messages []string
	repoWidth, msgWidth := 0, 0
	for _, e := range entries {
		msg := cut(e.Message, 40)
		if e.Commits > 1 {
			msg += fmt.Sprintf(" +%d", e.Commits-1)
		}
		messages = append(messages, msg)
		repoWidth = max(repoWidth, utf8.RuneCountInString(filepath.Base(e.Repo)))
		msgWidth = max(msgWidth, utf8.RuneCountInString(msg))
	}

	day, failed := "", 0
	for i, e := range entries {
		t := e.Time.Local()
		if d := dayName(t); d != day {
			if day != "" {
				fmt.Println()
			}
			day = d
			fmt.Println(paint("1", d))
		}

		mark := paint("32", "✓")
		if e.Error != "" {
			mark = paint("31", "✗")
			failed++
		}

		fmt.Printf("  %s  %s  %s  %s  %s  %s\n",
			paint("2", t.Format("15:04")),
			mark,
			paint("1", pad(filepath.Base(e.Repo), repoWidth)),
			paint("2", e.Commit),
			pad(messages[i], msgWidth),
			paint("2", fmt.Sprintf("%5s", e.Took.Round(time.Second))),
		)
		if e.Error != "" {
			fmt.Printf("            %s\n", paint("31", e.Error))
		}
	}

	word := "deploys"
	if len(entries) == 1 {
		word = "deploy"
	}
	fmt.Printf("\n%d %s, %d failed\n", len(entries), word, failed)
}

func dayName(t time.Time) string {
	now := time.Now()
	switch t.Format("2006-01-02") {
	case now.Format("2006-01-02"):
		return "Today"
	case now.AddDate(0, 0, -1).Format("2006-01-02"):
		return "Yesterday"
	}
	if t.Year() == now.Year() {
		return t.Format("Mon, Jan 2")
	}
	return t.Format("Mon, Jan 2 2006")
}

func cut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-3]) + "..."
}

func pad(s string, width int) string {
	return s + strings.Repeat(" ", max(0, width-utf8.RuneCountInString(s)))
}
