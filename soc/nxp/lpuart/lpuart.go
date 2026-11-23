// NXP Low Power Universal Asynchronous Receiver/Transmitter (LPUART) driver
// https://github.com/usbarmory/tamago
//
// Copyright (c) The TamaGo Authors. All Rights Reserved.
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

// Package lpuart implements a driver for NXP LPUART controllers adopting the
// following reference specifications:
//   - IMX93RM - i.MX 93 Applications Processor Reference Manual - Rev 4 2023/12
//
// This package is only meant to be used with `GOOS=tamago` as supported by the
// TamaGo framework for bare metal Go, see https://github.com/usbarmory/tamago.
package lpuart

import (
	"runtime"
	"sync"

	"github.com/usbarmory/tamago/internal/reg"
)

// LPUART registers
const (
	DEFAULT_BAUDRATE = 115200

	LPUART_GLOBAL = 0x08
	GLOBAL_RST    = 1

	LPUART_BAUD   = 0x10
	BAUD_OSR      = 24
	BAUD_BOTHEDGE = 17
	BAUD_SBNS     = 13
	BAUD_SBR      = 0

	LPUART_STAT = 0x14
	STAT_TDRE   = 23
	STAT_TC     = 22
	STAT_RDRF   = 21
	STAT_OR     = 19
	STAT_NF     = 18
	STAT_FE     = 17
	STAT_PF     = 16

	LPUART_CTRL = 0x18
	CTRL_TE     = 19
	CTRL_RE     = 18
	CTRL_RIE    = 21
	CTRL_M      = 4
	CTRL_PE     = 1

	LPUART_DATA = 0x1c
	DATA_RXEMPT = 12

	LPUART_FIFO  = 0x28
	FIFO_TXFLUSH = 15
	FIFO_RXFLUSH = 14
	FIFO_TXFE    = 7
	FIFO_RXFE    = 3

	LPUART_WATER = 0x2c
)

// Oversampling ratio boundaries
const (
	minOSR = 4
	maxOSR = 32
	maxSBR = 0x1fff
)

// LPUART represents a serial port instance.
type LPUART struct {
	sync.Mutex

	// Controller index
	Index int
	// Base register
	Base uint32
	// Clock gate register (LPCG), optional
	LPCG uint32
	// Clock retrieval function
	Clock func() uint32
	// Interrupt ID
	IRQ int
	// Port speed
	Baudrate uint32

	// control registers
	global uint32
	baud   uint32
	stat   uint32
	ctrl   uint32
	data   uint32
	fifo   uint32
	water  uint32

	rx chan bool
}

// Init initializes and enables the LPUART for 8N1 operation.
func (hw *LPUART) Init() {
	hw.Lock()

	if hw.Base == 0 || hw.Clock == nil {
		panic("invalid LPUART controller instance")
	}

	if hw.Baudrate == 0 {
		hw.Baudrate = DEFAULT_BAUDRATE
	}

	hw.global = hw.Base + LPUART_GLOBAL
	hw.baud = hw.Base + LPUART_BAUD
	hw.stat = hw.Base + LPUART_STAT
	hw.ctrl = hw.Base + LPUART_CTRL
	hw.data = hw.Base + LPUART_DATA
	hw.fifo = hw.Base + LPUART_FIFO
	hw.water = hw.Base + LPUART_WATER

	if hw.LPCG != 0 {
		// enable clock
		reg.Set(hw.LPCG, 0)
	}

	hw.setup()

	// no defer as goos.Hwinit1 might call us from system stack
	hw.Unlock()
}

// baudDivisors returns the oversampling ratio (OSR) and baud rate modulo
// divisor (SBR) which best approximate the requested baud rate:
//
//	baudrate = clock / (OSR * SBR)
func baudDivisors(clock uint32, baudrate uint32) (osr uint32, sbr uint32) {
	var minErr uint64 = ^uint64(0)

	for o := uint32(minOSR); o <= maxOSR; o++ {
		s := (clock + (o*baudrate)/2) / (o * baudrate)

		if s == 0 || s > maxSBR {
			continue
		}

		actual := uint64(clock) / uint64(o*s)

		var e uint64

		if actual > uint64(baudrate) {
			e = actual - uint64(baudrate)
		} else {
			e = uint64(baudrate) - actual
		}

		if e <= minErr {
			minErr = e
			osr = o
			sbr = s
		}
	}

	return
}

func (hw *LPUART) setup() {
	osr, sbr := baudDivisors(hw.Clock(), hw.Baudrate)

	if osr == 0 {
		panic("unsupported LPUART baud rate")
	}

	// wait for any pending transmission (e.g. bootloader output)
	if reg.Get(hw.ctrl, CTRL_TE) {
		for !reg.Get(hw.stat, STAT_TC) {
			// no Gosched as goos.Hwinit1 might call us from
			// system stack
		}
	}

	// software reset
	reg.Set(hw.global, GLOBAL_RST)
	reg.Clear(hw.global, GLOBAL_RST)

	baud := (osr-1)<<BAUD_OSR | sbr<<BAUD_SBR

	// sample on both edges with low oversampling ratios
	if osr < 8 {
		baud |= 1 << BAUD_BOTHEDGE
	}

	reg.Write(hw.baud, baud)

	// 8 data bits, no parity, 1 stop bit
	reg.Write(hw.ctrl, 0)

	// enable and flush FIFOs, set watermarks to 0
	reg.Write(hw.fifo, 1<<FIFO_TXFE|1<<FIFO_RXFE|1<<FIFO_TXFLUSH|1<<FIFO_RXFLUSH)
	reg.Write(hw.water, 0)

	// clear status flags
	reg.Write(hw.stat, 1<<STAT_OR|1<<STAT_NF|1<<STAT_FE|1<<STAT_PF)

	// enable transmitter and receiver
	reg.Write(hw.ctrl, 1<<CTRL_TE|1<<CTRL_RE)
}

// Enable enables the LPUART transmitter and receiver, this is only required
// after an explicit disable (see Disable()) as initialized interfaces (see
// Init()) are enabled by default.
func (hw *LPUART) Enable() {
	reg.Set(hw.ctrl, CTRL_TE)
	reg.Set(hw.ctrl, CTRL_RE)
}

// Disable disables the LPUART transmitter and receiver.
func (hw *LPUART) Disable() {
	reg.Clear(hw.ctrl, CTRL_TE)
	reg.Clear(hw.ctrl, CTRL_RE)
}

// EnableInterrupt enables interrupt generation for receive data. Once enabled
// [LPUART.Read] blocks until an interrupt is serviced with
// [LPUART.ServiceInterrupt].
func (hw *LPUART) EnableInterrupt() {
	if hw.rx != nil {
		return
	}

	hw.rx = make(chan bool, 1)
	reg.Set(hw.ctrl, CTRL_RIE)
}

// ServiceInterrupt services a receive data interrupt.
func (hw *LPUART) ServiceInterrupt() {
	// RDRF can only be cleared by draining the FIFO, therefore mask the
	// interrupt until the next Rx attempt.
	reg.Clear(hw.ctrl, CTRL_RIE)

	select {
	case hw.rx <- true:
	default:
	}
}

// Tx transmits a single character to the serial port.
func (hw *LPUART) Tx(c byte) {
	for reg.GetN(hw.stat, STAT_TDRE, 1) == 0 {
		// wait for TX FIFO to have room for a character
	}

	reg.Write(hw.data, uint32(c))
}

// Rx receives a single character from the serial port.
func (hw *LPUART) Rx() (c byte, valid bool) {
	stat := reg.Read(hw.stat)

	if stat&(1<<STAT_OR) != 0 {
		// clear overrun to resume reception
		reg.Write(hw.stat, 1<<STAT_OR)
	}

	if stat&(1<<STAT_RDRF) == 0 {
		return
	}

	data := reg.Read(hw.data)

	if data&(1<<DATA_RXEMPT) != 0 {
		return
	}

	if stat&(1<<STAT_NF|1<<STAT_FE|1<<STAT_PF) != 0 {
		reg.Write(hw.stat, 1<<STAT_NF|1<<STAT_FE|1<<STAT_PF)
		return
	}

	return byte(data & 0xff), true
}

// Write data from buffer to serial port.
func (hw *LPUART) Write(buf []byte) (n int, _ error) {
	for _, c := range buf {
		hw.Tx(c)
	}

	return len(buf), nil
}

// Read available data to buffer from serial port.
func (hw *LPUART) Read(buf []byte) (n int, _ error) {
	var valid bool

	for n < len(buf) {
		buf[n], valid = hw.Rx()

		if !valid {
			if n == 0 {
				if hw.rx != nil {
					reg.Set(hw.ctrl, CTRL_RIE)
					<-hw.rx
					continue
				}

				runtime.Gosched()
			}

			break
		}

		n++
	}

	return
}
