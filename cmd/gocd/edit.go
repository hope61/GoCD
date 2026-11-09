package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hope61/GoCD/internal/config"
)

func editConfig(path string) {
	in := bufio.NewReader(os.Stdin)
	editor, err := pickEditor(in)
	if err != nil {
		log.Fatal(err)
	}

	original, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatalf("failed to read config: %v", err)
	}

	tmp, err := os.CreateTemp("", "gocd-config-*.yaml")
	if err != nil {
		log.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmp.Name())
	tmp.Write(original)
	tmp.Close()

	for {
		cmd := exec.Command("sh", "-c", editor+` "$1"`, "sh", tmp.Name())
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			log.Fatalf("editor failed: %v", err)
		}

		edited, _ := os.ReadFile(tmp.Name())
		if bytes.Equal(edited, original) {
			fmt.Println("no changes made")
			return
		}

		if _, err := config.Load(tmp.Name()); err != nil {
			fmt.Printf("config has errors: %v\n", err)
			if ask(in, "edit again? [Y/n] ") == "n" {
				fmt.Println("changes discarded, config.yaml was not touched")
				return
			}
			continue
		}

		if err := os.WriteFile(path, edited, 0644); err != nil {
			log.Fatalf("failed to save config: %v (try with sudo)", err)
		}
		fmt.Printf("saved %s\nrestart to apply: sudo systemctl restart gocd\n", path)
		return
	}
}

func pickEditor(in *bufio.Reader) (string, error) {
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if e := os.Getenv(env); e != "" {
			return e, nil
		}
	}

	var found []string
	for _, e := range []string{"nvim", "vim", "nano"} {
		if _, err := exec.LookPath(e); err == nil {
			found = append(found, e)
		}
	}
	if len(found) == 0 {
		return "", errors.New("no editor found, install nano/vim/nvim or set $EDITOR")
	}

	fmt.Println("select an editor (set $EDITOR to skip this):")
	for i, e := range found {
		fmt.Printf("  %d. %s\n", i+1, e)
	}

	for {
		n, err := strconv.Atoi(ask(in, fmt.Sprintf("choose 1-%d: ", len(found))))
		if err == nil && n >= 1 && n <= len(found) {
			saveEditor(found[n-1])
			return found[n-1], nil
		}
	}
}

func saveEditor(editor string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	var rc string
	switch filepath.Base(os.Getenv("SHELL")) {
	case "bash":
		rc = filepath.Join(home, ".bashrc")
	case "zsh":
		rc = filepath.Join(home, ".zshrc")
	default:
		fmt.Printf("add 'export EDITOR=%s' to your shell config to skip this next time\n", editor)
		return
	}

	data, _ := os.ReadFile(rc)
	if strings.Contains(string(data), "export EDITOR=") {
		fmt.Printf("%s already sets EDITOR, run: source %s\n", rc, rc)
		return
	}

	f, err := os.OpenFile(rc, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("couldn't save EDITOR to %s: %v\n", rc, err)
		return
	}
	defer f.Close()

	fmt.Fprintf(f, "\nexport EDITOR=%s\n", editor)
	fmt.Printf("saved EDITOR=%s to %s, run: source %s\n", editor, rc, rc)
}

func ask(in *bufio.Reader, prompt string) string {
	fmt.Print(prompt)
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		log.Fatal("no input")
	}
	return strings.ToLower(strings.TrimSpace(line))
}
