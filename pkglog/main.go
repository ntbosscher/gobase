package pkglog

import (
	"github.com/ntbosscher/gobase/env"
	"io"
	"log"
	"os"
	"strings"
)

var DefaultLogLevel Level

var DefaultOutput io.Writer
var DefaultFlags = log.Lshortfile

func defaults() (Level, io.Writer, int) {
	levelStr := env.Optional("DEFAULT_LOG_LEVEL", "verbose")
	DefaultLogLevel = parseLevel(levelStr)

	defaultLogOutput := env.Optional("DEFAULT_LOG_OUTPUT", "stdout")
	switch defaultLogOutput {
	case "stdout":
		DefaultOutput = os.Stdout
	case "stderr":
		DefaultOutput = os.Stderr
	default:
		var err error
		DefaultOutput, err = os.OpenFile(defaultLogOutput, os.O_RDWR|os.O_CREATE, os.ModePerm)
		if err != nil {
			panic("unable to open log file '" + defaultLogOutput + "' (parsed from env DEFAULT_LOG_OUTPUT): " + err.Error())
		}
	}

	return DefaultLogLevel, DefaultOutput, DefaultFlags
}

type Level int

const (
	Verbose Level = iota
	Info
	Error
	None
)

func New(name string) *Logger {
	name = strings.ToLower(name)

	// convert "importer.excel" to "IMPORTER_EXCEL_LOGLEVEL"
	key := strings.ReplaceAll(strings.ToUpper(name), ".", "_") + "_LOG_LEVEL"

	level, output, flags := defaults()

	levelStr := env.Optional(key, "")
	if levelStr != "" {
		level = parseLevel(levelStr)
	}

	lg := &Logger{
		Name:   name,
		level:  level,
		output: output,
		flags:  flags,
	}

	lg.configure()
	return lg
}

func parseLevel(value string) Level {
	value = strings.ToLower(value)
	switch value {
	case "verbose":
		return Verbose
	case "info":
		return Info
	case "error":
		return Error
	case "none":
		return None
	}

	panic("unexpected log level '" + value + "', should be something like 'verbose', 'info', 'error', 'none'")
}

type Logger struct {
	Name   string
	level  Level
	output io.Writer
	flags  int

	Verbose *log.Logger
	Info    *log.Logger
	Error   *log.Logger
}

// SetLevel sets the log level
func (l *Logger) SetLevel(level Level) {
	l.level = level
	l.configure()
}

// SetFlags sets the log.New flags
func (l *Logger) SetFlags(flags int) {
	l.flags = flags
	l.configure()
}

func (l *Logger) SetOutput(output io.Writer) {
	l.output = output
	l.configure()
}

func (l *Logger) Print(v ...any) {
	l.Info.Print(v...)
}

func (l *Logger) Fatalln(v ...any) {
	l.Info.Fatalln(v...)
}

func (l *Logger) Fatalf(format string, v ...any) {
	l.Info.Fatalf(format, v...)
}

func (l *Logger) Printf(format string, v ...any) {
	l.Info.Printf(format, v...)
}

func (l *Logger) Println(v ...any) {
	l.Info.Println(v...)
}

func (l *Logger) configure() {
	l.Error = configure(l.Error, l.output, l.Name+":error: ", l.flags, l.level <= Error)
	l.Verbose = configure(l.Verbose, l.output, l.Name+":verbose: ", l.flags, l.level <= Verbose)
	l.Info = configure(l.Info, l.output, l.Name+":info: ", l.flags, l.level <= Info)
}

func configure(lg *log.Logger, output io.Writer, prefix string, flags int, enabled bool) *log.Logger {
	if lg == nil {
		lg = log.New(output, prefix, flags)
	}

	if enabled {
		lg.SetOutput(output)
	} else {
		lg.SetOutput(io.Discard)
	}

	lg.SetFlags(flags)
	lg.SetPrefix(prefix)

	return lg
}
