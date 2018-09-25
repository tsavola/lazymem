// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lazymem

import (
	"context"
	"fmt"
	"os"

	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseutil"
)

// Config for the memory filesystem.
type Config struct {
	Mountpoint string
	ErrorLog   Logger
	DebugLog   Logger
}

// Manager of lazy memory.  It is backed by a custom filesystem implementation.
type Manager struct {
	Config

	rmdir  bool
	fs     *fileSystem
	server fuse.Server
	mount  *fuse.MountedFileSystem
}

// New mounts a filesystem instance.
func New(ctx context.Context, config *Config) (m *Manager, err error) {
	m = new(Manager)

	if config != nil {
		m.Config = *config
	}

	if m.Mountpoint == "" {
		runDir := os.Getenv("XDG_RUNTIME_DIR")
		if runDir == "" {
			runDir = "/run"
		}
		m.Mountpoint = fmt.Sprintf("%s/lazymem/%d", runDir, os.Getpid())
	}

	err = os.MkdirAll(m.Mountpoint, 0700)
	if err == nil {
		m.rmdir = true
	} else if !os.IsExist(err) {
		return
	}

	m.fs = newFileSystem()
	m.server = fuseutil.NewFileSystemServer(m.fs)

	mountConfig := fuse.MountConfig{
		OpContext:   ctx,
		FSName:      "lazymem",
		Subtype:     "lazymem",
		ErrorLogger: adaptLogger(m.ErrorLog),
		DebugLogger: adaptLogger(m.DebugLog),
	}

	m.mount, err = fuse.Mount(m.Mountpoint, m.server, &mountConfig)
	if err != nil {
		m.cleanup()
	}
	return
}

// Shutdown unmounts the filesystem.
func (m *Manager) Shutdown(ctx context.Context) (err error) {
	err = fuse.Unmount(m.mount.Dir())

	if e := m.mount.Join(ctx); err == nil {
		err = e
	}

	if e := m.cleanup(); err == nil {
		err = e
	}
	return
}

func (m *Manager) cleanup() (err error) {
	if m.rmdir {
		err = os.Remove(m.Mountpoint)
	}
	return
}
