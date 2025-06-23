package telemetry

import (
	"fmt"
	"strings"

	"github.com/getsentry/sentry-go"
	logging "github.com/ipfs/go-log/v2"
)

// SentryLogger is a logger that sends errors messages to Sentry for error,
// panic and fatal logs.
type SentryLogger struct {
	log *logging.ZapEventLogger
}

func (s *SentryLogger) Debug(args ...any) {
	s.log.Debug(args...)
}

func (s *SentryLogger) Debugf(format string, args ...any) {
	s.log.Debugf(format, args...)
}

func (s *SentryLogger) Error(args ...any) {
	if logging.LogLevel(s.log.Level()) <= logging.LevelError {
		sentry.CaptureException(fmt.Errorf(formatString(len(args)), args...))
	}
	s.log.Error(args...)
}

func (s *SentryLogger) Errorf(format string, args ...any) {
	if logging.LogLevel(s.log.Level()) <= logging.LevelError {
		sentry.CaptureException(fmt.Errorf(format, args...))
	}
	s.log.Errorf(format, args...)
}

func (s *SentryLogger) Fatal(args ...any) {
	if logging.LogLevel(s.log.Level()) <= logging.LevelFatal {
		sentry.CaptureException(fmt.Errorf(formatString(len(args)), args...))
	}
	s.log.Fatal(args...)
}

func (s *SentryLogger) Fatalf(format string, args ...any) {
	if logging.LogLevel(s.log.Level()) <= logging.LevelFatal {
		sentry.CaptureException(fmt.Errorf(format, args...))
	}
	s.log.Fatalf(format, args...)
}

func (s *SentryLogger) Info(args ...any) {
	s.log.Info(args...)
}

func (s *SentryLogger) Infof(format string, args ...any) {
	s.log.Infof(format, args...)
}

func (s *SentryLogger) Panic(args ...any) {
	if logging.LogLevel(s.log.Level()) <= logging.LevelPanic {
		sentry.CaptureException(fmt.Errorf(formatString(len(args)), args...))
	}
	s.log.Panic(args...)
}

func (s *SentryLogger) Panicf(format string, args ...any) {
	if logging.LogLevel(s.log.Level()) <= logging.LevelPanic {
		sentry.CaptureException(fmt.Errorf(format, args...))
	}
	s.log.Panicf(format, args...)
}

func (s *SentryLogger) Warn(args ...any) {
	s.log.Warn(args...)
}

func (s *SentryLogger) Warnf(format string, args ...any) {
	s.log.Warnf(format, args...)
}

// NewSentryLogger returns a logger that sends errors messages to Sentry for
// error, panic and fatal logs.
//
// Note: you should call [sentry.Init] before using the returned logger.
func NewSentryLogger(system string) *SentryLogger {
	return &SentryLogger{log: logging.Logger(system)}
}

// formatString gets a format string for the specified number of arguments.
func formatString(n int) string {
	return strings.Repeat(" %+v", n)[1:]
}
