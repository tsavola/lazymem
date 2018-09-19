// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tester

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"image/png"
	"log"
	"os"
	"strconv"
	"syscall"
	"time"
)

var Tests = map[string]func([]string){
	"TestDelay": func(args []string) {
		mem, err := syscall.Mmap(0, 0, 256*4096, syscall.PROT_READ, syscall.MAP_PRIVATE)
		if err != nil {
			log.Fatal(err)
		}
		defer func() {
			if err := syscall.Munmap(mem); err != nil {
				log.Print(err)
			}
		}()

		for i := 0; i < 256; i++ {
			offset := i * 4096
			value := mem[offset]
			t := time.Now()

			fmt.Printf("%s: mem[0x%x] = %d\n", t, offset, value)

			if value != byte(i) {
				os.Exit(1)
			}
		}
	},

	"TestWritePrivate": func(args []string) {
		mem, err := syscall.Mmap(0, 0, 256*4096, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_PRIVATE)
		if err != nil {
			log.Fatal(err)
		}
		defer func() {
			if err := syscall.Munmap(mem); err != nil {
				log.Print(err)
			}
		}()

		for i := 0; i < 256*4096; i++ {
			mem[i] = byte(i)
		}
	},

	"TestHTTPGet": func(args []string) {
		length, err := strconv.Atoi(args[0])
		if err != nil {
			log.Fatal(err)
		}

		mem, err := syscall.Mmap(0, 0, length, syscall.PROT_READ, syscall.MAP_PRIVATE)
		if err != nil {
			log.Fatal(err)
		}
		defer syscall.Munmap(mem)

		image, err := jpeg.Decode(bytes.NewReader(mem))
		if err != nil {
			log.Fatal(err)
		}

		if filename := os.Getenv("TEST_HTTP_GET_OUTPUT"); filename != "" {
			f, err := os.Create(filename)
			if err != nil {
				log.Fatal(err)
			}
			defer f.Close()

			if err := png.Encode(f, image); err != nil {
				log.Fatal(err)
			}
		}
	},
}
