package logutil

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"time"
)

type Logger struct {
	name     string
	filename string
	writer   *log.Logger
}

var (
	loggerDefaultName = "default-logger"
	loggerDefaultFile = "log"
	loggerCache       = make(map[string]*Logger)
	loggerCurPath, _  = os.Getwd()
	loggerRootPath    = ""
)

func getLoggerFile(path string) string {
	now := time.Now().Format("2006-01-02")
	return loggerRootPath + path + "-" + now + ".log"
}

func newLogger(name string, filename string) *Logger {
	if loggerRootPath == "" {
		loggerRootPath = loggerCurPath + "/logs/"

		_, err := os.Stat(loggerRootPath)
		if err != nil && os.IsNotExist(err) {
			os.MkdirAll(loggerRootPath, os.ModePerm)
		}
	}
	var logger = new(Logger)
	logger.name = name
	logger.writer = log.New(os.Stdout, "", log.Ldate|log.Ltime)
	logger.filename = filename
	return logger
}

func GetDefaultLogger() *Logger {
	return GetLogger(loggerDefaultName, loggerDefaultFile)
}

func GetLogger(name string, filename string) *Logger {
	if logger, ok := loggerCache[name]; ok {
		return logger
	} else {
		logger = newLogger(name, filename)
		loggerCache[name] = logger
		return logger
	}
}

func (l *Logger) dolog(lv string, format string, callIdx int, a ...interface{}) {
	var logFileStr = getLoggerFile(l.filename)
	logFile, err := os.OpenFile(logFileStr, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	_, file, line, ok := runtime.Caller(callIdx)
	if !ok {
		return
	}
	var idx = strings.LastIndexByte(file, '/')
	var msg = fmt.Sprintf("%s:%d: ", file[idx+1:len(file)-3], line) + fmt.Sprintf(format, a...)
	l.writer.SetOutput(logFile)
	l.writer.SetPrefix(lv)
	l.writer.Println(msg)
	logFile.Close()
}

func (l *Logger) Info(format string, a ...interface{}) {
	go l.dolog("[INFO] ", format, 2, a...)
}

func (l *Logger) Warn(format string, a ...interface{}) {
	go l.dolog("[WARN] ", format, 2, a...)
}

func (l *Logger) Error(format string, a ...interface{}) {
	go l.dolog("[ERROR] ", format, 2, a...)
}

// var logger = GetDefaultLogger()

// func Info(format string, a ...interface{}) {
// 	go logger.dolog("[INFO] ", format, 2, a...)
// }

// func Warn(format string, a ...interface{}) {
// 	go logger.dolog("[WARN] ", format, 2, a...)
// }

// func Error(format string, a ...interface{}) {
// 	go logger.dolog("[ERROR] ", format, 2, a...)
// }
