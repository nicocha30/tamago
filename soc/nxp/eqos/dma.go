// NXP Ethernet Quality of Service (ENET_QOS) driver
// https://github.com/usbarmory/tamago
//
// Copyright (c) The TamaGo Authors. All Rights Reserved.
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

package eqos

import (
	"errors"

	"github.com/usbarmory/tamago/dma"
	"github.com/usbarmory/tamago/internal/reg"
)

const (
	MTU          = 1500
	maxFrameSize = MTU + 18

	// receive buffer size, must be a multiple of the bus width
	bufferSize  = 1600
	bufferAlign = 64

	descSize        = 16
	defaultRingSize = 16

	// DMA buffers might be mapped as device memory, which requires
	// aligned accesses.
	copyAlign = 8
)

// Descriptor fields (DES3)
const (
	DESC3_OWN   = 31
	DESC3_IOC   = 30 // read format (receive)
	DESC3_CTXT  = 30 // write-back format
	DESC3_FD    = 29
	DESC3_LD    = 28
	DESC3_BUF1V = 24 // read format (receive)
	DESC3_CE    = 24 // write-back format (receive), CRC error
	DESC3_OE    = 21 // write-back format (receive), overflow error
	DESC3_RE    = 20 // write-back format (receive), receive error
	DESC3_ES    = 15 // write-back format (receive)
	DESC3_GP    = 23 // write-back format (receive), giant packet

	DESC3_LENGTH_MASK = 0x7fff
)

type descriptorRing struct {
	// descriptors and buffers physical base addresses
	desc uint32
	addr uint32
	// buffers
	data []byte

	// data cache maintenance (cacheable data buffers only)
	cache Cache

	rx       bool
	stats    *Stats
	size     int
	index    int
	refilled bool
}

// copyDMA copies data from or to DMA buffers, ensuring aligned accesses.
func copyDMA(dst []byte, src []byte) (n int) {
	n = min(len(dst), len(src))
	r := n % copyAlign

	copy(dst[:n-r], src[:n-r])

	for i := n - r; i < n; i++ {
		dst[i] = src[i]
	}

	return
}

func (ring *descriptorRing) descriptor(i int) uint32 {
	return ring.desc + uint32(i*descSize)
}

func (ring *descriptorRing) buffer(i int) (addr uint32, buf []byte) {
	off := i * bufferSize
	return ring.addr + uint32(off), ring.data[off : off+bufferSize]
}

// arm prepares a receive descriptor for DMA processing.
func (ring *descriptorRing) arm(i int) {
	d := ring.descriptor(i)
	addr, _ := ring.buffer(i)

	reg.Write(d+0x0, addr)
	reg.Write(d+0x4, 0)
	reg.Write(d+0x8, 0)
	// ownership must be passed last (descriptors are device memory)
	reg.Write(d+0xc, 1<<DESC3_OWN|1<<DESC3_IOC|1<<DESC3_BUF1V)
}

func (ring *descriptorRing) init(rx bool, n int, buffers *dma.Region, cache Cache) uint32 {
	if buffers == nil {
		buffers = dma.Default()
	}

	ring.cache = cache
	ring.rx = rx
	ring.size = n
	ring.index = 0

	// To avoid excessive DMA region fragmentation, a single allocation
	// reserves all descriptors and another one all data buffers.
	desc, _ := dma.Reserve(n*descSize, bufferAlign)

	// Buffers are aligned to, and sized in multiples of, the cache line
	// size: cache maintenance on one never affects its neighbors.
	addr, data := buffers.Reserve(n*bufferSize, bufferAlign)

	if cache != nil {
		cache.InvalidateDataCache(addr, n*bufferSize)
	}

	ring.desc = uint32(desc)

	// DMA buffers might be mapped as device memory, where data cache
	// zeroing instructions (e.g. DC ZVA used by clear()) fault, therefore
	// descriptors are cleared with register writes.
	for i := uint32(0); i < uint32(n*descSize); i += 4 {
		reg.Write(ring.desc+i, 0)
	}

	ring.addr = uint32(addr)
	ring.data = data

	if rx {
		for i := range n {
			ring.arm(i)
		}
	}

	return ring.desc
}

// tail returns the descriptor tail pointer, which points past the last
// descriptor available to the DMA.
func (ring *descriptorRing) tail() uint32 {
	if ring.rx {
		// all descriptors are re-armed as soon as consumed
		return ring.descriptor(ring.size)
	}

	return ring.descriptor(ring.index)
}

func (ring *descriptorRing) next() {
	ring.index = (ring.index + 1) % ring.size
}

func (ring *descriptorRing) pop(buf []byte) (n int, err error) {
	ring.refilled = false

	d := ring.descriptor(ring.index)
	des3 := reg.Read(d + 0xc)

	if des3&(1<<DESC3_OWN) != 0 {
		return
	}

	i := ring.index
	ring.next()

	valid := des3&(1<<DESC3_CTXT) == 0 &&
		des3&(1<<DESC3_FD) != 0 &&
		des3&(1<<DESC3_LD) != 0 &&
		des3&(1<<DESC3_ES) == 0

	if des3&(1<<DESC3_ES) != 0 && ring.stats != nil {
		ring.stats.Errors++
		ring.stats.CRC += des3 >> DESC3_CE & 1
		ring.stats.Receive += des3 >> DESC3_RE & 1
		ring.stats.Overflow += des3 >> DESC3_OE & 1
		ring.stats.Giant += des3 >> DESC3_GP & 1
	}

	if valid {
		length := int(des3 & DESC3_LENGTH_MASK)

		if length > bufferSize {
			err = errors.New("invalid frame length")
		} else {
			addr, data := ring.buffer(i)

			if ring.cache != nil {
				// discard stale lines before reading what the
				// MAC received
				ring.cache.InvalidateDataCache(uint(addr), length)
			}

			n = copyDMA(buf, data[:length])
		}
	}

	// give the descriptor back to the DMA
	ring.arm(i)
	ring.refilled = true

	return
}

func (ring *descriptorRing) push(buf []byte) (err error) {
	d := ring.descriptor(ring.index)

	if reg.Read(d+0xc)&(1<<DESC3_OWN) != 0 {
		return errors.New("frame not sent")
	}

	addr, data := ring.buffer(ring.index)
	copyDMA(data, buf)

	if ring.cache != nil {
		// write the frame back to memory before the MAC reads it
		ring.cache.CleanDataCache(uint(addr), len(buf))
	}

	length := uint32(len(buf))

	reg.Write(d+0x0, addr)
	reg.Write(d+0x4, 0)
	reg.Write(d+0x8, length)
	// ownership must be passed last (descriptors are device memory)
	reg.Write(d+0xc, 1<<DESC3_OWN|1<<DESC3_FD|1<<DESC3_LD|length)

	ring.next()

	return
}
