// Copyright (c) 2018 Timo Savola. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sparse implements TemporalBuffer.
package sparse

import (
	"io"
	"sort"
	"sync"
)

type frame struct {
	offset int64
	data   []byte
}

type Buffer struct {
	lock   sync.Mutex
	cond   sync.Cond
	frames []frame
	finish bool
}

func NewBuffer() (b *Buffer) {
	b = new(Buffer)
	b.cond.L = &b.lock
	return
}

func (b *Buffer) searchForFrame(offset int64) int {
	return sort.Search(len(b.frames), func(i int) bool {
		return b.frames[i].offset >= offset
	})
}

func (b *Buffer) ReadAt(dest []byte, offset int64) (int, error) {
	var copied int

	b.lock.Lock()
	defer b.lock.Unlock()

	for len(dest) > 0 {
		data, err := b.getData(offset, len(dest))
		if err != nil {
			return copied, err
		}

		n := copy(dest, data)
		dest = dest[n:]
		offset += int64(n)
		copied += n
	}

	return copied, nil
}

// getData must be called with b.lock held.
func (b *Buffer) getData(offset int64, length int) ([]byte, error) {
	for {
		i := b.searchForFrame(offset)
		if i < len(b.frames) {
			f := &b.frames[i]
			if o := int(offset - f.offset); o >= 0 && o < len(f.data) {
				return b.sliceFrame(i, f, o, length), nil
			}
		}
		if i > 0 {
			f := &b.frames[i-1]
			if o := int(offset - f.offset); o >= 0 && o < len(f.data) {
				return b.sliceFrame(i-1, f, o, length), nil
			}
		}

		if b.finish {
			return nil, io.EOF
		}

		b.cond.Wait()
	}
}

// sliceFrame must be called with b.lock held.
func (b *Buffer) sliceFrame(i int, f *frame, o, resultLength int) (result []byte) {
	result = f.data[o:]
	if len(result) > resultLength {
		result = result[:resultLength]
	}

	if o == 0 {
		if len(f.data) == len(result) {
			// remove whole frame
			b.frames = append(b.frames[:i], b.frames[i+1:]...)
		} else {
			// remove beginning of frame
			f.offset += int64(len(result))
			f.data = f.data[len(result):]
		}
	} else {
		prefix := f.data[:o]
		suffix := f.data[o+len(result):]

		// remove middle and end of frame
		f.data = prefix

		if len(suffix) > 0 {
			// insert end of frame after its beginning
			newFrame := frame{
				offset: f.offset + int64(o+len(result)),
				data:   suffix,
			}
			b.frames = append(b.frames[:i], append([]frame{newFrame}, b.frames[i:]...)...)
		}
	}

	return
}

// ProduceFrame transfers ownership of the data object to the buffer.
func (b *Buffer) ProduceFrame(data []byte, offset int64) {
	b.lock.Lock()
	defer b.lock.Unlock()

	i := b.searchForFrame(offset)
	b.frames = append(b.frames[:i], append([]frame{frame{offset, data}}, b.frames[i:]...)...)
	b.cond.Broadcast()
}

// ProductionFinished indicates that no more frames will be produced, either
// because all have been produced, or due to cancellation or error.
func (b *Buffer) ProductionFinished() {
	b.lock.Lock()
	defer b.lock.Unlock()

	b.finish = true
	b.cond.Broadcast()
}
