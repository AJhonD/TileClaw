package log

import (
	"io"
	"os"
	"path/filepath"

	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/shiena/ansicolor"
	log "github.com/sirupsen/logrus"
)

const LogDir = "logs"

var (
	SysLog  = log.New()
	ProgLog = log.New()
)

func init() {
	Init()
}

func Init() {
	formatter := &nested.Formatter{
		HideKeys:        true,
		ShowFullLevel:   true,
		TimestampFormat: "2006-01-02 15:04:05.000",
	}

	SysLog.SetFormatter(formatter)
	SysLog.SetLevel(log.InfoLevel)
	sysFile, err := openLogFile("tileclaw.log")
	if err == nil {
		SysLog.SetOutput(io.MultiWriter(ansicolor.NewAnsiColorWriter(os.Stdout), sysFile))
	} else {
		SysLog.SetOutput(ansicolor.NewAnsiColorWriter(os.Stdout))
	}

	ProgLog.SetFormatter(formatter)
	ProgLog.SetLevel(log.InfoLevel)
	progFile, err := openLogFile("download.log")
	if err == nil {
		ProgLog.SetOutput(io.MultiWriter(os.Stdout, progFile))
	} else {
		ProgLog.SetOutput(os.Stdout)
	}
}

func openLogFile(name string) (*os.File, error) {
	if err := os.MkdirAll(LogDir, os.ModePerm); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(LogDir, name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
}
