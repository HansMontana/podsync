package logging

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// Logger writes concise, timestamped application logs for CLI operations.
type Logger struct {
	output    io.Writer
	now       func() time.Time
	component string
	mu        *sync.Mutex
}

func New(output io.Writer) *Logger {
	return &Logger{output: output, now: time.Now, mu: &sync.Mutex{}}
}

func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{output: l.output, now: l.now, component: component, mu: l.mu}
}

func (l *Logger) Info(message string) {
	l.write("INFO", message)
}

func (l *Logger) Warn(message string) {
	l.write("WARN", message)
}

func (l *Logger) Error(message string) {
	l.write("ERROR", message)
}

func (l *Logger) write(level, message string) {
	component := l.component
	if component == "" {
		component = "app"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = fmt.Fprintf(l.output, "%s\t%s\t%s\t%s\n", l.now().UTC().Format("2006-01-02T15:04:05.000Z"), level, component, message)
}
