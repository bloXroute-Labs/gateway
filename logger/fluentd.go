package logger

import (
	"github.com/evalphobia/logrus_fluent"
	"github.com/sirupsen/logrus"
	"strings"
)

// InitFluentD - initialise logging
func InitFluentD(fluentDEnabled bool, fluentDHost, nodeID string, logLevel logrus.Level) error {
	Formatter := new(logrus.TextFormatter)
	Formatter.TimestampFormat = timestampFormat
	Formatter.FullTimestamp = true

	if fluentDEnabled {
		hook, err := logrus_fluent.NewWithConfig(logrus_fluent.Config{
			Host:          fluentDHost,
			Port:          24224,
			MarshalAsJSON: true,
			AsyncConnect:  true,
		})
		if err != nil {
			logrus.Warnf("Failed to create fluentd config with error %v", err)
			return nil
		}

		var hookLevels []logrus.Level
		for _, level := range logrus.AllLevels {
			if int(level) <= int(logLevel) {
				hookLevels = append(hookLevels, level)
			}
		}
		hook.SetLevels(hookLevels)

		hook.SetTag(fluentDTag)
		hook.SetMessageField("msg")
		hook.AddCustomizer(func(entry *logrus.Entry, data logrus.Fields) {
			data["level"] = strings.ToUpper(entry.Level.String())
			data["timestamp"] = entry.Time.Format(Formatter.TimestampFormat)
			data["instance"] = nodeID
		})

		logrus.AddHook(hook)
		logrus.Infof("connection established with fluentd hook at %v:%v", hook.Fluent.FluentHost, hook.Fluent.FluentPort)
	}
	return nil
}
