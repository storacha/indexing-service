package telemetry

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/getsentry/sentry-go"
	logging "github.com/ipfs/go-log/v2"
	"github.com/storacha/indexing-service/pkg/internal/extmocks"
	"github.com/stretchr/testify/mock"
)

type MockSentry struct {
	mock.Mock
}

func (m *MockSentry) CaptureException(err error) *sentry.EventID {
	args := m.Called(err)
	id := sentry.EventID(args.String(0))
	return &id
}

func TestSentryLogger(t *testing.T) {
	testCases := []struct {
		method  string
		format  string
		args    []any
		capture bool
		level   logging.LogLevel
	}{
		{
			method: "Debug",
			args:   []any{"test"},
		},
		{
			method: "Debugf",
			format: "test %s",
			args:   []any{"arg"},
		},
		{
			method: "Info",
			args:   []any{"test"},
		},
		{
			method: "Infof",
			format: "test %s",
			args:   []any{"arg"},
		},
		{
			method: "Warn",
			args:   []any{"test"},
		},
		{
			method: "Warnf",
			format: "test %s",
			args:   []any{"arg"},
		},
		{
			method:  "Error",
			args:    []any{"boom"},
			capture: true,
		},
		{
			method:  "Error",
			args:    []any{"boom"},
			capture: false,
			level:   logging.LevelPanic,
		},
		{
			method:  "Errorf",
			format:  "boom %s",
			args:    []any{"arg"},
			capture: true,
		},
		{
			method:  "Errorf",
			format:  "boom %s",
			args:    []any{"arg"},
			capture: false,
			level:   logging.LevelPanic,
		},
		{
			method:  "Panic",
			args:    []any{"boom"},
			capture: true,
		},
		{
			method:  "Panicf",
			format:  "boom %s",
			args:    []any{"arg"},
			capture: true,
		},
		{
			method:  "Panic",
			args:    []any{"boom"},
			capture: false,
			level:   logging.LevelFatal,
		},
		{
			method:  "Panicf",
			format:  "boom %s",
			args:    []any{"arg"},
			capture: false,
			level:   logging.LevelFatal,
		},
		{
			method:  "Fatal",
			args:    []any{"boom"},
			capture: true,
		},
		{
			method:  "Fatalf",
			format:  "boom %s",
			args:    []any{"arg"},
			capture: true,
		},
		{
			method:  "Fatal",
			args:    []any{"boom"},
			capture: false,
			level:   logging.LogLevel(99),
		},
		{
			method:  "Fatalf",
			format:  "boom %s",
			args:    []any{"arg"},
			capture: false,
			level:   logging.LogLevel(99),
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s capture=%t level=%d", tc.method, tc.capture, tc.level), func(t *testing.T) {
			mockSentry := MockSentry{}
			mockLog := extmocks.NewMockEventLogger(t)
			system := fmt.Sprintf("test-%s", tc.method)
			log := SentryLogger{
				system:           system,
				log:              mockLog,
				captureException: mockSentry.CaptureException,
			}

			cfg := logging.GetConfig()
			cfg.SubsystemLevels[system] = tc.level
			logging.SetupLogging(cfg)

			var args []any
			if tc.format != "" {
				args = append(args, tc.format)
			}
			args = append(args, tc.args...)

			mockLog.On(tc.method, args...).Return()
			if tc.capture {
				if tc.format == "" {
					mockSentry.On("CaptureException", fmt.Errorf(formatString(len(tc.args)), tc.args...)).Return("eventID")
				} else {
					mockSentry.On("CaptureException", fmt.Errorf(tc.format, tc.args...)).Return("eventID")
				}
			}

			vals := make([]reflect.Value, len(args))
			for i, a := range args {
				vals[i] = reflect.ValueOf(a)
			}
			reflect.ValueOf(&log).MethodByName(tc.method).Call(vals)

			mockSentry.AssertExpectations(t)
			mockLog.AssertExpectations(t)
		})
	}
}
