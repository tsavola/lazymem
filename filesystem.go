// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lazymem

import (
	"context"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
)

var never = time.Now().Add(time.Hour * 24 * 365 * 200)

type fileSystem struct {
	fuseutil.NotImplementedFileSystem
	uid uint32
	gid uint32

	lock   sync.Mutex
	nodes  map[fuseops.InodeID]buffer
	names  map[string]fuseops.InodeID
	lastId fuseops.InodeID
}

func newFileSystem() *fileSystem {
	return &fileSystem{
		uid:    uint32(os.Getuid()),
		gid:    uint32(os.Getgid()),
		nodes:  make(map[fuseops.InodeID]buffer),
		names:  make(map[string]fuseops.InodeID),
		lastId: fuseops.RootInodeID,
	}
}

func (fs *fileSystem) registerBuffer(b buffer) (id fuseops.InodeID, name string) {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	id = fs.lastId + 1
	name = strconv.FormatUint(uint64(id), 36)

	fs.nodes[id] = b
	fs.names[name] = id
	fs.lastId = id
	return
}

func (fs *fileSystem) forgetBufferName(name string) {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	delete(fs.names, name)
}

func (fs *fileSystem) forgetBufferNode(id fuseops.InodeID) {
	fs.lock.Lock()
	defer fs.lock.Unlock()

	delete(fs.nodes, id)
}

func (fs *fileSystem) bufferAttributes(size int64) fuseops.InodeAttributes {
	return fuseops.InodeAttributes{
		Size: uint64(size),
		Mode: 0700,
		Uid:  fs.uid,
		Gid:  fs.gid,
	}
}

func (fs *fileSystem) LookUpInode(ctx context.Context, op *fuseops.LookUpInodeOp) (err error) {
	if op.Parent != fuseops.RootInodeID {
		return fuse.ENOENT
	}

	fs.lock.Lock()
	defer fs.lock.Unlock()

	id, found := fs.names[op.Name]
	if !found {
		return fuse.ENOENT
	}

	b := fs.nodes[id]

	op.Entry.Child = id
	op.Entry.Attributes = fs.bufferAttributes(b.size)
	op.Entry.AttributesExpiration = never
	return
}

func (fs *fileSystem) GetInodeAttributes(ctx context.Context, op *fuseops.GetInodeAttributesOp) (err error) {
	if op.Inode == fuseops.RootInodeID {
		op.Attributes = fuseops.InodeAttributes{
			Mode: 0500 | os.ModeDir,
			Uid:  fs.uid,
			Gid:  fs.gid,
		}
	} else {
		fs.lock.Lock()
		b, found := fs.nodes[op.Inode]
		fs.lock.Unlock()
		if !found {
			return fuse.ENOENT
		}

		op.Attributes = fs.bufferAttributes(b.size)
	}

	op.AttributesExpiration = never
	return
}

func (fs *fileSystem) SetInodeAttributes(context.Context, *fuseops.SetInodeAttributesOp) error {
	return nil
}

func (fs *fileSystem) OpenFile(ctx context.Context, op *fuseops.OpenFileOp) (err error) {
	fs.lock.Lock()
	_, found := fs.nodes[op.Inode]
	fs.lock.Unlock()
	if !found {
		return fuse.EIO
	}

	op.Handle = fuseops.HandleID(op.Inode)
	op.KeepPageCache = true
	return
}

func (fs *fileSystem) ReadFile(ctx context.Context, op *fuseops.ReadFileOp) (err error) {
	fs.lock.Lock()
	b, found := fs.nodes[op.Inode]
	fs.lock.Unlock()
	if !found {
		return fuse.ENOENT
	}

	op.BytesRead, err = b.readAt(adjustLen(op.Dst, op.Offset, b.size), op.Offset)
	return
}

func (fs *fileSystem) WriteFile(ctx context.Context, op *fuseops.WriteFileOp) (err error) {
	fs.lock.Lock()
	b, found := fs.nodes[op.Inode]
	fs.lock.Unlock()
	if !found {
		return fuse.ENOENT
	}

	_, err = b.writeAt(adjustLen(op.Data, op.Offset, b.size), op.Offset)
	return
}

func (fs *fileSystem) FlushFile(ctx context.Context, op *fuseops.FlushFileOp) (err error) {
	fs.lock.Lock()
	_, found := fs.nodes[op.Inode]
	fs.lock.Unlock()
	if !found {
		return fuse.ENOENT
	}

	return
}

func (fs *fileSystem) ReleaseFileHandle(ctx context.Context, op *fuseops.ReleaseFileHandleOp) (err error) {
	fs.lock.Lock()
	b, found := fs.nodes[fuseops.InodeID(op.Handle)]
	fs.lock.Unlock()
	if !found {
		return fuse.ENOENT
	}

	err = b.close()
	return
}

func (fs *fileSystem) ForgetInode(ctx context.Context, op *fuseops.ForgetInodeOp) (err error) {
	if op.Inode != fuseops.RootInodeID {
		fs.forgetBufferNode(op.Inode)
	}
	return
}

func noWriteAt([]byte, int64) (n int, err error) {
	err = fuse.ENOSYS
	return
}

func noClose() (err error) {
	return
}

func adjustLen(ioBuf []byte, ioOffset, availSize int64) []byte {
	if n := availSize - ioOffset; n < int64(len(ioBuf)) {
		ioBuf = ioBuf[:n]
	}
	return ioBuf
}
