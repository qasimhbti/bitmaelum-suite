package internal

import (
	"github.com/bitmaelum/bitmaelum-suite/internal/config"
	"github.com/sirupsen/logrus"
	prefixed "github.com/x-cray/logrus-prefixed-formatter"
	"os"
	"strings"
	"time"
)

// SetLogging will set the correct level and log path
func SetLogging(level, path string) {
	formatter := new(prefixed.TextFormatter)
	formatter.FullTimestamp = true
	formatter.TimestampFormat = time.Stamp
	logrus.SetFormatter(formatter)

	// Default to stderr
	logrus.SetOutput(os.Stderr)

	if path == "stdout" {
		logrus.SetOutput(os.Stdout)

	} else if path == "stderr" {
		logrus.SetOutput(os.Stderr)

	} else if strings.HasPrefix(path, "syslog") {
		// Default to localhost syslog daemon
		syslogHost := "localhost:514"

		splits := strings.SplitN(path, ":", 2)
		if len(splits) == 2 {
			syslogHost = splits[1]
		}

		hook, err := setupSyslogHook("udp", syslogHost)
		if err != nil {
			logrus.Error("Unable to connect to syslog daemon. Falling back to stderr")
			logrus.SetOutput(os.Stderr)
		} else {
			logrus.AddHook(hook)
		}
	} else {
		// Default to a path
		w, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0664)
		if err != nil {
			w = os.Stderr
		}

		logrus.SetOutput(w)
	}

	switch level {
	case "trace":
		logrus.SetLevel(logrus.TraceLevel)
		break
	case "debug":
		logrus.SetLevel(logrus.DebugLevel)
		break
	case "info":
		logrus.SetLevel(logrus.InfoLevel)
		break
	case "warning":
		logrus.SetLevel(logrus.WarnLevel)
		break
	case "error":
	default:
		logrus.SetLevel(logrus.ErrorLevel)
		break
	}

	logrus.Tracef("setting loglevel to '%s'", config.Server.Logging.Level)
}
