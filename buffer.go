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
	if remain := b.size - offset; remain < int64(len(dest)) {
		dest = dest[:remain]
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	for len(dest) > 0 {
		data := b.getData(ctx, offset)
		if data == nil {
			break
		}

		n := copy(dest, data)
		dest = dest[n:]
		offset += int64(n)
		copied += n
	}

	return
}

func (b *buffer) replaceData(offset int64, data []byte) {
	ctx := context.Background()

	if remain := b.size - offset; remain < int64(len(data)) {
		data = data[:remain]
	}

	b.lock.Lock()
	defer b.lock.Unlock()

	for len(data) > 0 {
		buf := b.getData(ctx, offset)
		if buf == nil {
			break
		}

		n := copy(buf, data)
		data = data[n:]
		offset += int64(n)
	}
}

func (b *buffer) getData(ctx context.Context, offset int64) []byte {
	for {
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

		select {
		case <-ctx.Done():
			return nil
		default:
		}

		b.cond.Wait()
	}
}

func (b *buffer) produceFrame(f Frame) {
	i := b.searchForFrame(f.Offset)

	b.lock.Lock()
	defer b.lock.Unlock()

	b.frames = append(b.frames[:i], append([]Frame{f}, b.frames[i:]...)...)
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
