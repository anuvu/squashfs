package main

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

type printfLogger struct {
	Verbosity int
}

func (p printfLogger) log(lvl int, f string, a ...interface{}) {
	if p.Verbosity >= lvl {
		fmt.Fprintf(os.Stderr, f+"\n", a...)
	}
}

func (p printfLogger) Info(fmt string, a ...interface{}) {
	p.log(1, fmt, a...)
}

func (p printfLogger) Verbose(fmt string, a ...interface{}) {
	p.log(2, fmt, a...)
}

func (p printfLogger) Debug(fmt string, a ...interface{}) {
	p.log(3, fmt, a...)
}
