package logger

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(log.LstdFlags)
	})
	return &buf
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out)
}

func writeLog(t *testing.T, dir, name string, lines ...string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBinaryDir(t *testing.T) {
	dir := BinaryDir()
	if !filepath.IsAbs(dir) {
		t.Fatalf("not absolute: %s", dir)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("not a folder: %s %v", dir, err)
	}
}

func TestSetupWritesToTodaysFile(t *testing.T) {
	dir := t.TempDir()
	captureLog(t)

	var f *os.File
	captureStdout(t, func() {
		f = Setup(dir)
		log.Printf("hello from the test")
	})
	f.Close()

	name := fmt.Sprintf("gocd_%s.log", time.Now().Format("2006-01-02"))
	data, err := os.ReadFile(filepath.Join(dir, "logs", name))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "hello from the test") {
		t.Fatalf("log line missing from %s: %q", name, data)
	}

	captureStdout(t, func() {
		f = Setup(dir)
		log.Printf("second line")
	})
	f.Close()
	data, _ = os.ReadFile(filepath.Join(dir, "logs", name))
	if !strings.Contains(string(data), "hello from the test") || !strings.Contains(string(data), "second line") {
		t.Fatalf("second Setup should append, got %q", data)
	}
}

func TestCleanOldLogs(t *testing.T) {
	dir := t.TempDir()
	buf := captureLog(t)
	old := time.Now().Add(-8 * 24 * time.Hour)

	oldLog := writeLog(t, dir, "gocd_2026-01-01.log", "old")
	newLog := writeLog(t, dir, "gocd_2026-11-15.log", "new")
	otherOld := writeLog(t, dir, "notes.txt", "keep me")
	oldNotLog := writeLog(t, dir, "gocd_backup.txt", "keep me too")
	for _, p := range []string{oldLog, otherOld, oldNotLog} {
		os.Chtimes(p, old, old)
	}

	CleanOldLogs(dir, 7*24*time.Hour)

	if _, err := os.Stat(oldLog); !os.IsNotExist(err) {
		t.Fatal("old log was not deleted")
	}
	for _, p := range []string{newLog, otherOld, oldNotLog} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("%s should be kept: %v", filepath.Base(p), err)
		}
	}
	if !strings.Contains(buf.String(), "deleted old log file: gocd_2026-01-01.log") {
		t.Fatalf("deletion not logged: %q", buf.String())
	}
}

func TestListLogFiles(t *testing.T) {
	dir := t.TempDir()
	writeLog(t, dir, "gocd_2026-11-02.log", "b")
	writeLog(t, dir, "gocd_2026-11-01.log", "a")
	writeLog(t, dir, "other.log", "x")
	writeLog(t, dir, "gocd_notes.txt", "x")

	files := listLogFiles(dir)
	if len(files) != 2 || filepath.Base(files[0]) != "gocd_2026-11-01.log" || filepath.Base(files[1]) != "gocd_2026-11-02.log" {
		t.Fatalf("got %v", files)
	}

	if files := listLogFiles(filepath.Join(dir, "missing")); len(files) != 0 {
		t.Fatalf("missing folder should give no files, got %v", files)
	}
}

func TestShowLogs(t *testing.T) {
	dir := t.TempDir()

	if out := captureStdout(t, func() { ShowLogs(dir, 50) }); !strings.Contains(out, "no log files found") {
		t.Fatalf("got %q", out)
	}

	logs := filepath.Join(dir, "logs")
	os.MkdirAll(logs, 0755)
	writeLog(t, logs, "gocd_2026-11-01.log", "a1", "a2", "a3")
	writeLog(t, logs, "gocd_2026-11-02.log", "b1", "b2")

	if out := captureStdout(t, func() { ShowLogs(dir, 4) }); out != "a2\na3\nb1\nb2\n" {
		t.Fatalf("last 4 lines across files: got %q", out)
	}
	if out := captureStdout(t, func() { ShowLogs(dir, 50) }); out != "a1\na2\na3\nb1\nb2\n" {
		t.Fatalf("all lines: got %q", out)
	}
}
