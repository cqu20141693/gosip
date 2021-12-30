package log

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/cqu20141693/go-service-common/config"
	"github.com/cqu20141693/go-service-common/file"
	"github.com/cqu20141693/go-service-common/logger/cclog"
	"github.com/sirupsen/logrus"
	prefixed "github.com/x-cray/logrus-prefixed-formatter"
)

func TestFile(t *testing.T) {
	stderr := os.Stderr
	logTest(stderr)

	config.Default("cc.log.max-age", 3)
	config.Default("cc.log.rotate-time", "24h")
	rotateTime := config.GetString("cc.log.rotate-time")
	maxAge := config.GetInt64("cc.log.max-age")
	var path string
	if logDir := config.GetStringOrDefault("cc.log.dir", ""); logDir != "" {
		if strings.Contains(logDir, "/") {
			path = logDir
		} else {
			path = file.GetCurrentPath() + string(os.PathSeparator) + logDir
		}
	} else {
		path = file.GetCurrentPath()
	}

	service := config.GetStringOrDefault("cc.application.name", "service")
	writer, err := cclog.GetWriter(path, service+".log", rotateTime, maxAge)
	if err != nil {
		cclog.Error("rotate writer create failed")
		return
	}
	logTest(writer)

}

func logTest(writer io.Writer) {
	logger := &logrus.Logger{
		Out:          writer,
		Formatter:    new(logrus.TextFormatter),
		Hooks:        make(logrus.LevelHooks),
		Level:        logrus.InfoLevel,
		ExitFunc:     os.Exit,
		ReportCaller: false,
	}
	logger.Formatter = &prefixed.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05.000",
	}
	logrusLogger := NewLogrusLogger(logger, "main", nil)
	logrusLogger.WithPrefix("ccsip")
	logrusLogger.Info("test", "1")
}
