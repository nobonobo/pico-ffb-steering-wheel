//go:build debug

package logger

import "log"

var (
	Debugf  = log.Printf
	Debug   = log.Print
	Debugln = log.Println
)
