package main

import (
	"fmt"
	"os"
)

// Logger - basic logging interface
type Logger interface {
	Info(string, ...interface{}) (int, error)
	Verbose(string, ...interface{}) (int, error)
	Debug(string, ...interface{}) (int, error)
}

type printfLogger struct {
	Verbosity int
}

func (p printfLogger) log(lvl int, f string, a ...interface{}) (int, error) {
	if p.Verbosity >= lvl {
		return fmt.Fprintf(os.Stderr, f+"\n", a...)
	}
	return 0, nil
}

func (p printfLogger) Info(fmt string, a ...interface{}) (int, error) {
	return p.log(1, fmt, a...)
}

func (p printfLogger) Verbose(fmt string, a ...interface{}) (int, error) {
	return p.log(2, fmt, a...)
}

func (p printfLogger) Debug(fmt string, a ...interface{}) (int, error) {
	return p.log(3, fmt, a...)
}
