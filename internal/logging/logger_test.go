package logging

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLoggerFormatsReadableTimestampedLines(t *testing.T) {
	var output bytes.Buffer
	logger := &Logger{
		output: &output,
		now: func() time.Time {
			return time.Date(2026, 8, 17, 10, 51, 28, 508000000, time.FixedZone("UTC", 0))
		},
		component: "sync",
		mu:        &sync.Mutex{},
	}

	logger.Info("Starting sync")
	logger.Warn("Keeping existing file")
	logger.Error("Sync failed")

	want := "2026-08-17T10:51:28.508Z\tINFO\tsync\tStarting sync\n" +
		"2026-08-17T10:51:28.508Z\tWARN\tsync\tKeeping existing file\n" +
		"2026-08-17T10:51:28.508Z\tERROR\tsync\tSync failed\n"
	if output.String() != want {
		t.Fatalf("got %q, want %q", output.String(), want)
	}
}

func TestLoggerComponentsShareSynchronizedOutput(t *testing.T) {
	var output bytes.Buffer
	base := &Logger{output: &output, now: time.Now, mu: &sync.Mutex{}}
	syncLogger := base.WithComponent("sync")
	deviceLogger := base.WithComponent("device")

	var writers sync.WaitGroup
	for i := 0; i < 20; i++ {
		writers.Add(2)
		go func() {
			defer writers.Done()
			syncLogger.Info("sync message")
		}()
		go func() {
			defer writers.Done()
			deviceLogger.Warn("device message")
		}()
	}
	writers.Wait()

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 40 {
		t.Fatalf("got %d log lines, want 40", len(lines))
	}
	for _, line := range lines {
		if !strings.Contains(line, "Z\t") || (!strings.Contains(line, "\tsync\t") && !strings.Contains(line, "\tdevice\t")) {
			t.Fatalf("malformed log line %q", line)
		}
	}
}
