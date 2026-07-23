package logging_test

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"cpa-usage-keeper/internal/config"
	"cpa-usage-keeper/internal/logging"
	gormlogger "gorm.io/gorm/logger"
)

func TestGORMLoggerMapsFrameworkLevelsToLogrus(t *testing.T) {
	output := captureConsole(t, config.Config{LogLevel: "debug"}, func() {
		logger := logging.NewGORMLogger().LogMode(gormlogger.Info)
		logger.Info(context.Background(), "gorm info %s", "message")
		logger.Warn(context.Background(), "gorm warning")
		logger.Error(context.Background(), "gorm error")
	})

	plain := ansiPattern.ReplaceAllString(output, "")
	for _, want := range []string{
		"| info  | gorm info message",
		"| warn  | gorm warning",
		"| error | gorm error",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("expected GORM level output %q, got %q", want, plain)
		}
	}
}

func TestGORMLoggerReportsSlowQueryWithStructuredFields(t *testing.T) {
	output := captureConsole(t, config.Config{LogLevel: "info"}, func() {
		logger := logging.NewGORMLogger()
		logger.Trace(context.Background(), time.Now().Add(-300*time.Millisecond), func() (string, int64) {
			return "SELECT * FROM users", 3
		}, nil)
	})

	plain := ansiPattern.ReplaceAllString(output, "")
	want := regexp.MustCompile(`\| warn  \| gorm slow query \| elapsed=[^ ]+ rows=3 sql="SELECT \* FROM users" threshold=200ms\n$`)
	if !want.MatchString(plain) {
		t.Fatalf("expected structured slow query log, got %q", plain)
	}
}

func TestGORMLoggerReportsQueryErrorsThroughLogrus(t *testing.T) {
	output := captureConsole(t, config.Config{LogLevel: "info"}, func() {
		logger := logging.NewGORMLogger()
		logger.Trace(context.Background(), time.Now(), func() (string, int64) {
			return "SELECT broken", -1
		}, errors.New("query failed"))
	})

	plain := ansiPattern.ReplaceAllString(output, "")
	want := regexp.MustCompile(`\| error \| gorm query failed \| elapsed=[^ ]+ error="query failed" rows=-1 sql="SELECT broken"\n$`)
	if !want.MatchString(plain) {
		t.Fatalf("expected structured GORM error log, got %q", plain)
	}
}

func TestGORMLoggerDoesNotExpandSQLForFilteredLevels(t *testing.T) {
	queryCalls := 0
	output := captureConsole(t, config.Config{LogLevel: "error"}, func() {
		logger := logging.NewGORMLogger()
		logger.Trace(context.Background(), time.Now().Add(-300*time.Millisecond), func() (string, int64) {
			queryCalls++
			return "SELECT * FROM users", 3
		}, nil)
	})

	if queryCalls != 0 {
		t.Fatalf("expected filtered slow query to stay lazy, query calls=%d", queryCalls)
	}
	if output != "" {
		t.Fatalf("expected filtered slow query to stay silent, got %q", output)
	}
}
