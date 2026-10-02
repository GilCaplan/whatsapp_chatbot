package wa

import (
	"fmt"
	"os"
	"sync"
	"time"

	waLog "go.mau.fi/whatsmeow/util/log"
)

// Log levels for stderrLogger.
const (
	levelDebug = iota
	levelInfo
	levelWarn
	levelError
)

var levelNames = [...]string{"DEBUG", "INFO", "WARN", "ERROR"}

var stderrMu sync.Mutex

// stderrLogger is a waLog.Logger that writes to stderr (the server redirects
// stderr to logs/server.log). waLog.Stdout would pollute stdout.
type stderrLogger struct {
	mod string
	min int
}

func newLogger(module string, min int) waLog.Logger { return &stderrLogger{mod: module, min: min} }

func (l *stderrLogger) output(level int, msg string, args ...any) {
	if level < l.min {
		return
	}
	line := fmt.Sprintf("%s [%s %s] %s\n", time.Now().Format("2006-01-02 15:04:05.000"), l.mod, levelNames[level], fmt.Sprintf(msg, args...))
	stderrMu.Lock()
	os.Stderr.WriteString(line)
	stderrMu.Unlock()
}

func (l *stderrLogger) Debugf(msg string, args ...any) { l.output(levelDebug, msg, args...) }
func (l *stderrLogger) Infof(msg string, args ...any)  { l.output(levelInfo, msg, args...) }
func (l *stderrLogger) Warnf(msg string, args ...any)  { l.output(levelWarn, msg, args...) }
func (l *stderrLogger) Errorf(msg string, args ...any) { l.output(levelError, msg, args...) }
func (l *stderrLogger) Sub(mod string) waLog.Logger {
	return &stderrLogger{mod: l.mod + "/" + mod, min: l.min}
}
