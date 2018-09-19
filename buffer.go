// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lazymem

import (
	"context"
	"sort"
	"sync"
)

// Frame of memory contents.
type Frame struct {
	Offset int64
	Data   []byte
}

type buffer struct {
	size int64

	lock   sync.Mutex
	cond   sync.Cond
	frames []Frame
	noMore bool
}

func newBuffer(size int64) (b *buffer) {
	b = &buffer{
		size: size,
	}
	b.cond.L = &b.lock
	return
}

func (b *buffer) searchForFrame(offset int64) int {
	return sort.Search(len(b.frames), func(i int) bool {
		return b.frames[i].Offset >= offset
	})
}

func (b *buffer) copyData(ctx context.Context, dest []byte, offset int64) (copied int) {
	if tail := b.size - offset; tail < int64(len(dest)) {
		dest = dest[:tail]
	}

	for len(dest) > 0 {
		b := b.getData(ctx, offset)
		if b == nil {
			break
		}

		n := copy(dest, b)
		dest = dest[n:]
		offset += int64(n)
		copied += n
	}

	return
}

func (b *buffer) getData(ctx context.Context, offset int64) []byte {
	b.lock.Lock()
	defer b.lock.Unlock()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		i := b.searchForFrame(offset)
		if i < len(b.frames) {
			f := b.frames[i]
			if o := offset - f.Offset; o >= 0 && o < int64(len(f.Data)) {
				return f.Data[o:]
			}
		}
		if i > 0 {
			f := b.frames[i-1]
			if o := offset - f.Offset; o >= 0 && o < int64(len(f.Data)) {
				return f.Data[o:]
			}
		}

		if b.noMore {
			return nil
		}

		b.cond.Wait()
	}
}

func (b *buffer) produceFrame(f Frame) {
	i := b.searchForFrame(f.Offset)
	tail := append([]Frame{f}, b.frames[i:]...)

	b.lock.Lock()
	defer b.lock.Unlock()

	b.frames = append(b.frames[:i], tail...)
	b.cond.Broadcast()
}

func (b *buffer) noMoreFrames() {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.noMore = true
	b.cond.Broadcast()
}

func buffering(b *buffer, data <-chan Frame) {
	defer b.noMoreFrames()

	for f := range data {
		b.produceFrame(f)
	}
}
