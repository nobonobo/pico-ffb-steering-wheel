//go:build !debug

package logger

var (
	Debugf  = func(fmt string, args ...any) {}
	Debug   = func(args ...any) {}
	Debugln = func(args ...any) {}
)
