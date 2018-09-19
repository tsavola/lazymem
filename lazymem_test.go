// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lazymem_test

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/tsavola/lazymem"
	_ "github.com/tsavola/lazymem/internal/tester" // cache workaround
)

var goBin string

func init() {
	goBin = os.Getenv("GO")
	if goBin == "" {
		goBin = "/usr/local/bin/go"
	}
}

func runTester(t *testing.T, fd int, args ...string) {
	t.Helper()

	args = append([]string{goBin, "run", "internal/runtester.go", t.Name()}, args...)

	pid, err := syscall.ForkExec(goBin, args, &syscall.ProcAttr{
		Env:   syscall.Environ(),
		Files: []uintptr{uintptr(fd), 1, 2},
	})
	if err != nil {
		t.Fatal(err)
	}

	var status syscall.WaitStatus

	_, err = syscall.Wait4(pid, &status, 0, nil)
	if err != nil {
		t.Fatal(err)
	}

	if status.Exited() {
		if code := status.ExitStatus(); code != 0 {
			t.Fatal("tester error code:", code)
		}
	} else {
		t.Fatal("tester status:", status)
	}
	return

}

func newConfig(t *testing.T) (config *lazymem.Config) {
	t.Helper()

	config = &lazymem.Config{
		ErrorLogger: log.New(os.Stderr, t.Name()+": ERROR: ", 0),
	}

	if testing.Verbose() {
		config.DebugLogger = log.New(os.Stderr, t.Name()+": ", 0)
	}

	return
}

func TestDelay(t *testing.T) {
	ctx := context.Background()

	mm, err := lazymem.New(ctx, newConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := mm.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	}()

	data := make(chan lazymem.Frame)

	go func() {
		defer close(data)

		for i := 0; i < 256; i++ {
			time.Sleep(time.Millisecond)

			b := make([]byte, 4096)
			b[0] = byte(i)

			data <- lazymem.Frame{
				Offset: int64(i) * 4096,
				Data:   b,
			}
		}
	}()

	fd, err := mm.Create(256*4096, syscall.O_RDONLY, data)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := syscall.Close(fd); err != nil {
			t.Error(err)
		}
	}()

	runTester(t, fd)
}

func TestWritePrivate(t *testing.T) {
	ctx := context.Background()

	mm, err := lazymem.New(ctx, newConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := mm.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	}()

	data := make(chan lazymem.Frame)

	go func() {
		defer close(data)

		data <- lazymem.Frame{
			Data: make([]byte, 256*4096),
		}
	}()

	fd, err := mm.Create(256*4096, syscall.O_RDWR, data)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := syscall.Close(fd); err != nil {
			t.Error(err)
		}
	}()

	runTester(t, fd)
}

func TestHTTPGet(t *testing.T) {
	url := os.Getenv("TEST_HTTP_GET")
	if url == "" {
		t.Skip("TEST_HTTP_GET=url not specified")
	}

	ctx := context.Background()

	mm, err := lazymem.New(ctx, newConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	defer mm.Shutdown(ctx)

	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.ContentLength <= 0 {
		t.Fatal(resp.ContentLength)
	}

	data := make(chan lazymem.Frame)

	go func() {
		defer close(data)

		for offset := int64(0); offset < resp.ContentLength; offset += 131072 {
			readlen := resp.ContentLength - offset
			if readlen > 131072 {
				readlen = 131072
			}

			b := make([]byte, 131072)

			if _, err := io.ReadFull(resp.Body, b[:readlen]); err != nil {
				t.Error(err)
				return
			}

			data <- lazymem.Frame{
				Offset: offset,
				Data:   b,
			}
		}
	}()

	fd, err := mm.Create(resp.ContentLength, syscall.O_RDONLY, data)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd)

	runTester(t, fd, strconv.Itoa(int(resp.ContentLength)))
}
