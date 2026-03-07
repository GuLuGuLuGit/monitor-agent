package logger

import (
	"os"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// LogEntry 用于上报的日志条目
type LogEntry struct {
	Level   string
	Source  string
	Message string
	Time    string
	Sequence int64
}

var (
	globalLogger *zap.Logger
	globalSugar  *zap.SugaredLogger
	sequence     int64
	entries      []LogEntry
	entriesMu    sync.Mutex
	maxEntries   int = 1000
)

// Init 初始化全局 logger（写文件 + 控制台，并缓冲用于上报）
func Init(level, logFile string, maxSize, maxBackups int) error {
	var core zapcore.Core
	enc := zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
		MessageKey:  "msg",
		LevelKey:    "level",
		TimeKey:     "ts",
		EncodeLevel: zapcore.CapitalColorLevelEncoder,
	})

	// 控制台
	consoleCore := zapcore.NewCore(enc, zapcore.Lock(zapcore.AddSync(os.Stdout)), parseLevel(level))
	cores := []zapcore.Core{consoleCore}

	// 文件（可选）
	if logFile != "" {
		fileCore := zapcore.NewCore(
			zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
			zapcore.AddSync(&lumberjack.Logger{
				Filename:   logFile,
				MaxSize:    maxSize,
				MaxBackups: maxBackups,
			}),
			parseLevel(level),
		)
		cores = append(cores, fileCore)
	}

	core = zapcore.NewTee(cores...)
	globalLogger = zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	globalSugar = globalLogger.Sugar()
	return nil
}

func parseLevel(s string) zapcore.Level {
	switch s {
	case "DEBUG":
		return zapcore.DebugLevel
	case "INFO":
		return zapcore.InfoLevel
	case "WARN":
		return zapcore.WarnLevel
	case "ERROR":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

// Sync 刷新缓冲
func Sync() {
	if globalLogger != nil {
		_ = globalLogger.Sync()
	}
}

// Debug / Info / Warn / Error 带缓冲写入（用于上报）
func buffer(level, source, msg string, args []interface{}) {
	seq := atomic.AddInt64(&sequence, 1)
	entriesMu.Lock()
	defer entriesMu.Unlock()
	if len(entries) >= maxEntries {
		entries = entries[1:]
	}
	entries = append(entries, LogEntry{
		Level:    level,
		Source:   source,
		Message:  msg,
		Time:     time.Now().UTC().Format(time.RFC3339),
		Sequence: seq,
	})
}

func Debug(msg string, keysAndValues ...interface{}) {
	if globalSugar != nil {
		globalSugar.Debugw(msg, keysAndValues...)
	}
	buffer("DEBUG", "agent", msg, keysAndValues)
}

func Info(msg string, keysAndValues ...interface{}) {
	if globalSugar != nil {
		globalSugar.Infow(msg, keysAndValues...)
	}
	buffer("INFO", "agent", msg, keysAndValues)
}

func Warn(msg string, keysAndValues ...interface{}) {
	if globalSugar != nil {
		globalSugar.Warnw(msg, keysAndValues...)
	}
	buffer("WARN", "agent", msg, keysAndValues)
}

func Error(msg string, keysAndValues ...interface{}) {
	if globalSugar != nil {
		globalSugar.Errorw(msg, keysAndValues...)
	}
	buffer("ERROR", "agent", msg, keysAndValues)
}

// DrainEntries 取出并清空缓冲的日志条目（用于上报）
func DrainEntries(max int) []LogEntry {
	entriesMu.Lock()
	defer entriesMu.Unlock()
	if max <= 0 || len(entries) <= max {
		out := entries
		entries = nil
		return out
	}
	out := entries[:max]
	entries = entries[max:]
	return out
}

// EntryCount 当前缓冲条数
func EntryCount() int {
	entriesMu.Lock()
	defer entriesMu.Unlock()
	return len(entries)
}
