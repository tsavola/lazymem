// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lazymem_test

import (
	"context"
	"io"
	"net/http"
	"os"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/tsavola/lazymem"
	_ "github.com/tsavola/lazymem/internal/tester" // cache workaround
	"github.com/tsavola/lazymem/linear"
	"github.com/tsavola/lazymem/sparse"
)

var goBin string

func init() {
	goBin = os.Getenv("GO")
	if goBin == "" {
		goBin = "/usr/local/bin/go"
	}
}

type tOrB interface {
	Helper()
	Logf(string, ...interface{})
	Fatal(...interface{})
}

func runTester(t tOrB, testName string, fd int, args ...string) {
	t.Helper()

	args = append([]string{goBin, "run", "internal/runtester.go", testName}, args...)

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

type testLogger struct{ tOrB }

func (l testLogger) Printf(format string, v ...interface{}) { l.tOrB.Logf(format, v...) }

func newConfig(t tOrB, debug bool) (config *lazymem.Config) {
	t.Helper()

	config = &lazymem.Config{
		ErrorLog: testLogger{t},
	}

	if debug {
		config.DebugLog = testLogger{t}
	}

	return
}

func TestDelay(t *testing.T) {
	ctx := context.Background()

	mm, err := lazymem.New(ctx, newConfig(t, testing.Verbose()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := mm.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	}()

	buf := sparse.Buf()
	fd, err := mm.CreateTemporal(256*4096, syscall.O_RDONLY, buf)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := syscall.Close(fd); err != nil {
			t.Error(err)
		}
	}()

	go func() {
		defer buf.ProductionFinished()

		for i := 0; i < 256; i++ {
			time.Sleep(time.Millisecond)

			b := make([]byte, 4096)
			b[0] = byte(i)

			buf.ProduceFrame(b, int64(i)*4096)
		}
	}()

	runTester(t, t.Name(), fd)
}

func TestWritePrivate(t *testing.T) { testWrite(t, syscall.MAP_PRIVATE) }
func TestWriteShared(t *testing.T)  { testWrite(t, syscall.MAP_SHARED) }

func testWrite(t *testing.T, flags int) {
	t.Helper()

	ctx := context.Background()

	mm, err := lazymem.New(ctx, newConfig(t, testing.Verbose()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := mm.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	}()

	buf := linear.Buf(make([]byte, 256*4096))
	defer func() {
		<-buf.Closed()
	}()

	fd, err := mm.Create(int64(buf.Len()), syscall.O_RDWR, buf)
	if err != nil {
		buf.Close()
		t.Fatal(err)
	}
	defer func() {
		if err := syscall.Close(fd); err != nil {
			t.Error(err)
		}
	}()

	go func() {
		defer buf.PopulationFinished()
		b := buf.Bytes()

		for i := 0; i < buf.Len()/linear.BlockSize; i++ {
			time.Sleep(100 * time.Millisecond)

			for j := 0; j < linear.BlockSize; j++ {
				b[j] = byte(i + j)
			}

			b = b[linear.BlockSize:]
			buf.BlockPopulated(i)
		}
	}()

	runTester(t, "TestWrite", fd, strconv.Itoa(flags))
}

func TestHTTPGet(t *testing.T) {
	url := os.Getenv("TEST_HTTP_GET")
	if url == "" {
		t.Skip("TEST_HTTP_GET=url not specified")
	}

	ctx := context.Background()

	mm, err := lazymem.New(ctx, newConfig(t, testing.Verbose()))
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

	buf := sparse.Buf()
	fd, err := mm.CreateTemporal(resp.ContentLength, syscall.O_RDONLY, buf)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd)

	go func() {
		defer buf.ProductionFinished()

		var offset int64
		for offset < resp.ContentLength {
			n := int64(150023)

			if remain := resp.ContentLength - offset; remain < n {
				n = remain
			}

			b := make([]byte, n)

			if _, err := io.ReadFull(resp.Body, b); err != nil {
				t.Error(err)
				break
			}

			buf.ProduceFrame(b, offset)
			offset += int64(len(b))
		}
	}()

	runTester(t, t.Name(), fd, strconv.Itoa(int(resp.ContentLength)))
}
