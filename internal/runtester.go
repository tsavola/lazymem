// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"

	"github.com/tsavola/lazymem/internal/tester"
)

func main() {
	name := os.Args[1]
	test, found := tester.Tests[name]
	if found {
		test(os.Args[2:])
	} else {
		panic(name)
	}
}
