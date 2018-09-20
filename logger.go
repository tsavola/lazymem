// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lazymem

import (
	"log"
)

// Logger for error and/or debug messages.  Subset of log.Logger.
type Logger interface {
	Printf(format string, v ...interface{})
}

type logWriter struct {
	Logger
}

func (w logWriter) Write(b []byte) (n int, err error) {
	w.Printf("%s", string(b))
	n = len(b)
	return
}

func adaptLogger(x Logger) (l *log.Logger) {
	if x == nil {
		return
	}

	l, ok := x.(*log.Logger)
	if ok {
		return
	}

	l = log.New(logWriter{x}, "", 0)
	return
}
