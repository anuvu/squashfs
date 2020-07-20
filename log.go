package squashfs

import (
	"fmt"
	"os"
)

// Logger - basic logging interface
type Logger interface {
	Info(string, ...interface{})
	Verbose(string, ...interface{})
	Debug(string, ...interface{})
}

// PrintfLogger - logger that calls Fprintf(os.Stderr, ...)
type PrintfLogger struct {
	Verbosity int
}

func (p PrintfLogger) log(lvl int, f string, a ...interface{}) {
	if p.Verbosity >= lvl {
		fmt.Fprintf(os.Stderr, f+"\n", a...)
	}
}

func (p PrintfLogger) Info(fmt string, a ...interface{}) {
	p.log(1, fmt, a...)
}

func (p PrintfLogger) Verbose(fmt string, a ...interface{}) {
	p.log(2, fmt, a...)
}

func (p PrintfLogger) Debug(fmt string, a ...interface{}) {
	p.log(3, fmt, a...)
}
