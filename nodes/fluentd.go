package nodes

import (
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/evalphobia/logrus_fluent"
	log "github.com/sirupsen/logrus"
	"strings"
)

// InitFluentD - initialise logging
func InitFluentD(fluentDEnabled bool, fluentDHost string, nodeID types.NodeID) error {

	Formatter := new(log.TextFormatter)
	Formatter.TimestampFormat = "2006-01-02T15:04:05.000000"
	Formatter.FullTimestamp = true

	if fluentDEnabled {
		hook, err := logrus_fluent.NewWithConfig(logrus_fluent.Config{
			Host:          fluentDHost,
			Port:          24224,
			MarshalAsJSON: true,
			AsyncConnect:  true,
		})
		if err != nil {
			log.Warnf("Failed to create fluentd config with error %v", err)
			return nil
		}
		hook.SetLevels([]log.Level{
			log.PanicLevel,
			log.FatalLevel,
			log.ErrorLevel,
			log.WarnLevel,
			log.InfoLevel,
		})
		hook.SetTag("bx.go.log")
		hook.SetMessageField("msg")
		hook.AddCustomizer(func(entry *log.Entry, data log.Fields) {
			data["level"] = strings.ToUpper(entry.Level.String())
			data["timestamp"] = entry.Time.Format(Formatter.TimestampFormat)
			data["instance"] = nodeID
		})

		log.AddHook(hook)
		log.Infof("connection established with fluentd hook at %v:%v", hook.Fluent.FluentHost, hook.Fluent.FluentPort)
	}
	return nil
}
