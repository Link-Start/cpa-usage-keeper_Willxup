package logging

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	gormlogger "gorm.io/gorm/logger"
)

const gormSlowQueryThreshold = 200 * time.Millisecond

type gormLogrusLogger struct {
	level         gormlogger.LogLevel
	slowThreshold time.Duration
}

// NewGORMLogger 保留 GORM 默认的 warn 阈值，并把实际输出统一交给 Logrus。
func NewGORMLogger() gormlogger.Interface {
	return gormLogrusLogger{
		level:         gormlogger.Warn,
		slowThreshold: gormSlowQueryThreshold,
	}
}

func (l gormLogrusLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	l.level = level
	return l
}

func (l gormLogrusLogger) Info(ctx context.Context, message string, data ...interface{}) {
	if l.level < gormlogger.Info {
		return
	}
	logrus.WithContext(ctx).Infof(message, data...)
}

func (l gormLogrusLogger) Warn(ctx context.Context, message string, data ...interface{}) {
	if l.level < gormlogger.Warn {
		return
	}
	logrus.WithContext(ctx).Warnf(message, data...)
}

func (l gormLogrusLogger) Error(ctx context.Context, message string, data ...interface{}) {
	if l.level < gormlogger.Error {
		return
	}
	logrus.WithContext(ctx).Errorf(message, data...)
}

func (l gormLogrusLogger) Trace(ctx context.Context, begin time.Time, query func() (string, int64), queryErr error) {
	if l.level == gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	// 只有确定需要输出时才展开 SQL，保持 GORM 正常查询路径的原有开销边界。
	switch {
	case queryErr != nil && l.level >= gormlogger.Error && logrus.IsLevelEnabled(logrus.ErrorLevel):
		sql, rows := query()
		logrus.WithContext(ctx).WithError(queryErr).WithFields(logrus.Fields{
			"elapsed": elapsed.Round(time.Microsecond).String(),
			"rows":    rows,
			"sql":     sql,
		}).Error("gorm query failed")
	case l.slowThreshold > 0 && elapsed > l.slowThreshold && l.level >= gormlogger.Warn && logrus.IsLevelEnabled(logrus.WarnLevel):
		sql, rows := query()
		logrus.WithContext(ctx).WithFields(logrus.Fields{
			"elapsed":   elapsed.Round(time.Microsecond).String(),
			"rows":      rows,
			"sql":       sql,
			"threshold": l.slowThreshold.String(),
		}).Warn("gorm slow query")
	case l.level >= gormlogger.Info && logrus.IsLevelEnabled(logrus.InfoLevel):
		sql, rows := query()
		logrus.WithContext(ctx).WithFields(logrus.Fields{
			"elapsed": elapsed.Round(time.Microsecond).String(),
			"rows":    rows,
			"sql":     sql,
		}).Info("gorm query")
	}
}
