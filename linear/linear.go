// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package linear implements ClonedBuffer and SharedBuffer.
package linear

import (
	"io"
	"sync"
)

const BlockSize = 131072

type Buffer struct {
	linear []byte
	closed chan struct{}

	lock   sync.Mutex
	cond   sync.Cond
	bitmap []uint64
	finish bool
}

func NewBuffer(linear []byte) (b *Buffer) {
	bitLen := (len(linear) + BlockSize - 1) / BlockSize
	wordLen := (bitLen + 63) / 64

	b = &Buffer{
		linear: linear,
		closed: make(chan struct{}),
		bitmap: make([]uint64, wordLen),
	}
	b.cond.L = &b.lock
	return
}

func (b *Buffer) Bytes() []byte           { return b.linear }
func (b *Buffer) Len() int                { return len(b.linear) }
func (b *Buffer) Closed() <-chan struct{} { return b.closed }

func (b *Buffer) ReadAt(target []byte, sourceOffset int64) (n int, err error) {
	if !b.waitForBlocks(sourceOffset, len(target)) {
		err = io.EOF
		return
	}

	n = copy(target, b.linear[sourceOffset:])
	return
}

func (b *Buffer) waitForBlocks(offset int64, length int) bool {
	begin := uint((offset + BlockSize - 1) / BlockSize)
	end := uint((offset + int64(length) + BlockSize - 1) / BlockSize)

	b.lock.Lock()
	defer b.lock.Unlock()

	for {
		if b.checkForBlocks(begin, end) {
			return true
		}
		if b.finish {
			return false
		}

		b.cond.Wait()
	}
}

func (b *Buffer) checkForBlocks(begin, end uint) bool {
	for i := begin; i < end; i++ {
		if b.bitmap[i/64]&(1<<(i&63)) == 0 {
			return false
		}
	}
	return true
}

func (b *Buffer) WriteAt(source []byte, targetOffset int64) (n int, err error) {
	n = copy(b.linear[targetOffset:], source)
	return
}

func (b *Buffer) Close() (err error) {
	close(b.closed)
	return
}

// BlockPopulated marks a 128 kB block as available for reading.
func (b *Buffer) BlockPopulated(index int) {
	word := uint(index) / 64
	mask := uint64(1) << uint(index&63)

	if word >= uint(len(b.bitmap)) {
		panic(index)
	}

	b.lock.Lock()
	b.bitmap[word] |= mask
	b.lock.Unlock()

	b.cond.Broadcast()
}

// BlocksPopulated marks adjacent 128 kB blocks as available for reading.
func (b *Buffer) BlocksPopulated(index, count int) {
	begin := uint(index) / 64
	end := uint(index+count) / 64

	if begin > end {
		panic("negative block count")
	}
	if end > uint(len(b.bitmap)) {
		panic("block index or count out of bounds")
	}

	b.lock.Lock()
	for i := index; i < index+count; i++ {
		b.bitmap[i/64] |= 1 << uint(i&63)
	}
	b.lock.Unlock()

	b.cond.Broadcast()
}

// PopulationFinished indicates that no more blocks will become available,
// either because all have been populated, or due to cancellation or error.
func (b *Buffer) PopulationFinished() {
	b.lock.Lock()
	b.finish = true
	b.lock.Unlock()

	b.cond.Broadcast()
}
