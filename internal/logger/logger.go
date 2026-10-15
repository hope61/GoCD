package logger

import (
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
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
