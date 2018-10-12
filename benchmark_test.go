// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lazymem_test

import (
	"context"
	"reflect"
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"github.com/tsavola/lazymem"
	"github.com/tsavola/lazymem/internal/tester"
	"github.com/tsavola/lazymem/linear"
)

func BenchmarkSharedReadLazymem(b *testing.B)  { benchmarkSharedLazymem(b, "BenchmarkSharedRead") }
func BenchmarkSharedReadMemfd(b *testing.B)    { benchmarkSharedMemfd(b, "BenchmarkSharedRead") }
func BenchmarkSharedWriteLazymem(b *testing.B) { benchmarkSharedLazymem(b, "BenchmarkSharedWrite") }
func BenchmarkSharedWriteMemfd(b *testing.B)   { benchmarkSharedMemfd(b, "BenchmarkSharedWrite") }

func benchmarkSharedLazymem(b *testing.B, name string) {
	ctx := context.Background()

	mm, err := lazymem.New(ctx, newConfig(b, false))
	if err != nil {
		b.Fatal(err)
	}
	defer mm.Shutdown(ctx)

	data := make([]byte, tester.BenchmarkSize)

	for i := 0; i < b.N; i++ {
		func() {
			buf := linear.NewBuffer(data)
			buf.BlocksPopulated(0, len(data)/linear.BlockSize)
			buf.PopulationFinished()

			fd, err := mm.Create(tester.BenchmarkSize, syscall.O_RDWR, buf)
			if err != nil {
				b.Fatal(err)
			}
			defer syscall.Close(fd)

			runTester(b, name, fd)
		}()
	}
}

func benchmarkSharedMemfd(b *testing.B, name string) {
	data := make([]byte, tester.BenchmarkSize)

	nameBuf := []byte{0}
	namePtr := (*reflect.StringHeader)(unsafe.Pointer(&nameBuf)).Data

	for i := 0; i < b.N; i++ {
		func() {
			fd, _, errno := syscall.Syscall(319, namePtr, 1, 0)
			if errno != 0 {
				b.Fatal(errno)
			}
			defer syscall.Close(int(fd))

			if _, err := syscall.Write(int(fd), data); err != nil {
				b.Fatal(err)
			}

			runTester(b, name, int(fd))
		}()
	}

	runtime.KeepAlive(nameBuf)
}
