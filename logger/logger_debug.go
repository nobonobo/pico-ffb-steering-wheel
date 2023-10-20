//go:build debug

package logger

import "encoding/hex"

const DEBUG = true

var (
	Debugln = func(args ...any) {
		for i, v := range args {
			switch vv := v.(type) {
			case []byte:
				print(hex.EncodeToString(vv))
			default:
				print(v)
			}
			if i == len(args)-1 {
				println()
			} else {
				print(" ")
			}
		}
	}
)
