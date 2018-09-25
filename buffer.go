// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lazymem

import (
	"io"
	"path"
	"syscall"
)

// TemporalBuffer's content will be read at most once (per range).
type TemporalBuffer interface {
	io.ReaderAt
}

// ClonedBuffer's content may be read repeatedly.
type ClonedBuffer interface {
	io.ReaderAt
	io.Closer
}

// SharedBuffer's content may be overwritten.  A range won't be written to
// before it has been read at least once.
type SharedBuffer interface {
	io.ReaderAt
	io.WriterAt
	io.Closer
}

type buffer struct {
	size    int64
	readAt  func(target []byte, sourceOffset int64) (n int, err error)
	writeAt func(source []byte, targetOffset int64) (n int, err error)
	close   func() error
}

// Create a file descriptor which should be passed to another process for
// memory mapping.  The memory can be mapped multiple times as PROT_SHARED
// and/or PROT_PRIVATE.
func (m *Manager) Create(size int64, mode int, b SharedBuffer) (fd int, err error) {
	return m.create(buffer{size, b.ReadAt, b.WriteAt, b.Close}, mode)
}

// CreateCloned memory file descriptor which should be passed to another
// process for mapping.  The memory can be mapped multiple times as
// PROT_PRIVATE.
func (m *Manager) CreateCloned(size int64, mode int, b ClonedBuffer) (fd int, err error) {
	return m.create(buffer{size, b.ReadAt, noWriteAt, b.Close}, mode)
}

// CreateTemporal memory file descriptor which should be passed to another
// process for mapping.  The memory can be mapped once as PROT_PRIVATE.
func (m *Manager) CreateTemporal(size int64, mode int, b TemporalBuffer) (fd int, err error) {
	return m.create(buffer{size, b.ReadAt, noWriteAt, noClose}, mode)
}

func (m *Manager) create(b buffer, mode int) (fd int, err error) {
	id, name := m.fs.registerBuffer(b)
	fd, err = syscall.Open(path.Join(m.Mountpoint, name), mode, 0)
	m.fs.forgetBufferName(name)
	if err != nil {
		m.fs.forgetBufferNode(id)
		b.close()
	}
	return
}
