package telemetry

import (
	"fmt"
	"strings"

	"github.com/getsentry/sentry-go"
	logging "github.com/ipfs/go-log/v2"
)

type SentryExceptionCaptureFunc func(err error) *sentry.EventID

// SentryLogger is a logger that sends errors messages to Sentry for error,
// panic and fatal logs.
type SentryLogger struct {
	system           string
	log              logging.EventLogger
	captureException SentryExceptionCaptureFunc
}

func (s *SentryLogger) Debug(args ...any) {
	s.log.Debug(args...)
}

func (s *SentryLogger) Debugf(format string, args ...any) {
	s.log.Debugf(format, args...)
}

func (s *SentryLogger) Error(args ...any) {
	if getLevel(s.system) <= logging.LevelError {
		s.captureException(fmt.Errorf(formatString(len(args)), args...))
	}
	s.log.Error(args...)
}

func (s *SentryLogger) Errorf(format string, args ...any) {
	if getLevel(s.system) <= logging.LevelError {
		s.captureException(fmt.Errorf(format, args...))
	}
	s.log.Errorf(format, args...)
}

func (s *SentryLogger) Fatal(args ...any) {
	if getLevel(s.system) <= logging.LevelFatal {
		s.captureException(fmt.Errorf(formatString(len(args)), args...))
	}
	s.log.Fatal(args...)
}

func (s *SentryLogger) Fatalf(format string, args ...any) {
	if getLevel(s.system) <= logging.LevelFatal {
		s.captureException(fmt.Errorf(format, args...))
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
	if getLevel(s.system) <= logging.LevelPanic {
		s.captureException(fmt.Errorf(formatString(len(args)), args...))
	}
	s.log.Panic(args...)
}

func (s *SentryLogger) Panicf(format string, args ...any) {
	if getLevel(s.system) <= logging.LevelPanic {
		s.captureException(fmt.Errorf(format, args...))
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
	return &SentryLogger{
		system:           system,
		log:              logging.Logger(system),
		captureException: sentry.CaptureException,
	}
}

// formatString gets a format string for the specified number of arguments.
func formatString(n int) string {
	return strings.Repeat(" %+v", n)[1:]
}

// getLevel gets the configured log level for the passed subsystem.
func getLevel(system string) logging.LogLevel {
	cfg := logging.GetConfig()
	lvl, ok := cfg.SubsystemLevels[system]
	if !ok {
		return cfg.Level
	}
	return lvl
}
