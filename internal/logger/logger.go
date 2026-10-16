package logger

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func BinaryDir() string {
	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("failed to locate binary path: %v", err)
	}
	return filepath.Dir(exe)
}

func Setup(dir string) *os.File {
	logsDir := filepath.Join(dir, "logs")

	err := os.MkdirAll(logsDir, 0755)
	if err != nil {
		log.Fatalf("failed to create logs directory: %v", err)
	}

	CleanOldLogs(logsDir, 7*24*time.Hour)

	filename := fmt.Sprintf("gocd_%s.log", time.Now().Format("2006-01-02"))
	logPath := filepath.Join(logsDir, filename)

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("failed to open log file %s: %v", logPath, err)
	}

	multi := io.MultiWriter(os.Stdout, f)
	log.SetOutput(multi)
	log.SetFlags(log.Ldate | log.Ltime)

	return f
}

func ShowLogs(dir string, n int) {
	logsDir := filepath.Join(dir, "logs")

	var logFiles []string
	filepath.WalkDir(logsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasPrefix(d.Name(), "gocd_") && strings.HasSuffix(d.Name(), ".log") {
			logFiles = append(logFiles, path)
		}
		return nil
	})

	if len(logFiles) == 0 {
		fmt.Println("no log files found")
		return
	}

	sort.Strings(logFiles)

	var lines []string
	for i := len(logFiles) - 1; i >= 0 && len(lines) < n; i-- {
		fileLines, err := readLines(logFiles[i])
		if err != nil {
			continue
		}
		lines = append(fileLines, lines...)
	}

	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	for _, line := range lines {
		fmt.Println(line)
	}
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

func CleanOldLogs(dir string, maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)

	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasPrefix(d.Name(), "gocd_") || !strings.HasSuffix(d.Name(), ".log") {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		if info.ModTime().Before(cutoff) {
			os.Remove(path)
			log.Printf("deleted old log file: %s", d.Name())
		}

		return nil
	})
}
