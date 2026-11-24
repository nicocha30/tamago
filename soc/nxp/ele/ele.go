// NXP EdgeLock Enclave (ELE) driver
// https://github.com/usbarmory/tamago
//
// Copyright (c) The TamaGo Authors. All Rights Reserved.
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

// Package ele implements a driver for the NXP EdgeLock Enclave (ELE) security
// subsystem, accessed through its Messaging Unit (MU), adopting the following
// reference specifications:
//   - IMX93RM - i.MX 93 Applications Processor Reference Manual - Rev 4 2023/12
//   - EdgeLock Enclave API as implemented in U-Boot and OP-TEE for i.MX 93
//
// This package is only meant to be used with `GOOS=tamago` as supported by the
// TamaGo framework for bare metal Go, see https://github.com/usbarmory/tamago.
package ele

import (
	"encoding/binary"
	"errors"
	"strconv"
	"sync"
	"unsafe"

	"github.com/usbarmory/tamago/internal/reg"
)

// Messaging Unit registers
const (
	MU_PAR = 0x004
	PAR_RR = 8
	PAR_TR = 0

	MU_TCR = 0x120
	MU_TSR = 0x124
	MU_RCR = 0x128
	MU_RSR = 0x12c

	MU_TR = 0x200
	MU_RR = 0x280
)

// ELE message protocol
const (
	VERSION_BASELINE = 0x06
	VERSION_HSM      = 0x07

	REQUEST_TAG  = 0x17
	RESPONSE_TAG = 0xe1

	STATUS_SUCCESS = 0xd6

	CMD_GET_EVENTS = 0xa2
	CMD_START_RNG  = 0xa3
	CMD_TRNG_STATE = 0xa4
	CMD_RNG_GET    = 0xcd
	CMD_GET_INFO   = 0xda

	TRNG_STATE_READY = 0x3

	RNG_FLAGS_NO_RESEED = 0x0000

	// maximum message size in words (header included)
	maxMsgSize = 18
	// polling iterations before declaring a timeout
	maxPoll = 100_000_000
)

// Info represents the response to the ELE GET_INFO command.
type Info struct {
	// SoC identifier (e.g. 0x9300 for i.MX 93)
	SocID uint16
	// SoC silicon revision (e.g. 0xa1 for revision A1)
	SocRev uint16
	// Lifecycle state
	Lifecycle uint16
	// Unique identifier
	UID [16]byte
}

// Error represents an ELE communication or command error.
//
// As the ELE is used early in the runtime initialization (see
// runtime/goos.InitRNG), where package fmt cannot be used, errors are
// formatted only on demand.
type Error struct {
	// Command code (0 for transport errors)
	Command uint8
	// Error description
	Reason string
	// Associated value (e.g. response code)
	Value uint32
}

// Error implements the error interface.
func (e *Error) Error() string {
	s := "ELE "

	if e.Command != 0 {
		s += "command 0x" + strconv.FormatUint(uint64(e.Command), 16) + " "
	}

	return s + e.Reason + " (0x" + strconv.FormatUint(uint64(e.Value), 16) + ")"
}

// Cache represents the CPU data cache maintenance operations required to
// share buffers with the ELE.
type Cache interface {
	CleanDataCache(addr uint, size int)
	InvalidateDataCache(addr uint, size int)
}

// ELE represents the EdgeLock Enclave instance.
type ELE struct {
	sync.Mutex

	// Messaging Unit base register
	Base uint32
	// Data cache maintenance
	Cache Cache

	tr int
	rr int
}

const (
	cacheLine = 64
	bufSize   = 256
)

// Buffer shared with the ELE, its location is static to allow use before the
// Go runtime (and the dma package) is fully initialized. As TamaGo adopts a
// flat memory model its virtual address matches its physical one.
var shared [bufSize + cacheLine]byte

func sharedBuffer() (addr uint, buf []byte) {
	addr = uint(uintptr(unsafe.Pointer(&shared[0])))
	off := (cacheLine - addr%cacheLine) % cacheLine

	return addr + off, shared[off : off+bufSize]
}

// Init initializes the Messaging Unit used to communicate with the ELE.
func (hw *ELE) Init() {
	hw.Lock()
	defer hw.Unlock()

	if hw.Base == 0 {
		panic("invalid ELE instance")
	}

	par := reg.Read(hw.Base + MU_PAR)
	hw.tr = int(par >> PAR_TR & 0xff)
	hw.rr = int(par >> PAR_RR & 0xff)

	// reset status registers
	reg.Write(hw.Base+MU_TCR, 0)
	reg.Write(hw.Base+MU_RCR, 0)
}

func wait(addr uint32, pos int) bool {
	for i := 0; i < maxPoll; i++ {
		if reg.Get(addr, pos) {
			return true
		}
	}

	return false
}

func (hw *ELE) send(msg []uint32) error {
	if hw.tr == 0 {
		return errors.New("ELE not initialized")
	}

	for i, w := range msg {
		n := i % hw.tr

		if !wait(hw.Base+MU_TSR, n) {
			return errors.New("ELE transmit timeout")
		}

		reg.Write(hw.Base+MU_TR+uint32(4*n), w)
	}

	return nil
}

func (hw *ELE) receive() (rsp []uint32, err error) {
	if !wait(hw.Base+MU_RSR, 0) {
		return nil, errors.New("ELE receive timeout")
	}

	hdr := reg.Read(hw.Base + MU_RR)
	size := int(hdr >> 8 & 0xff)

	if size == 0 || size > maxMsgSize {
		return nil, &Error{Reason: "invalid response size", Value: uint32(size)}
	}

	rsp = make([]uint32, size)
	rsp[0] = hdr

	for i := 1; i < size; i++ {
		n := i % hw.rr

		if !wait(hw.Base+MU_RSR, n) {
			return nil, errors.New("ELE receive timeout")
		}

		rsp[i] = reg.Read(hw.Base + MU_RR + uint32(4*n))
	}

	return
}

// Call sends a command to the ELE and returns its response payload (header
// excluded).
func (hw *ELE) Call(version uint8, cmd uint8, payload ...uint32) (rsp []uint32, err error) {
	hw.Lock()
	defer hw.Unlock()

	return hw.call(version, cmd, payload...)
}

func (hw *ELE) call(version uint8, cmd uint8, payload ...uint32) (rsp []uint32, err error) {
	size := 1 + len(payload)

	if size > maxMsgSize {
		return nil, errors.New("ELE message too large")
	}

	msg := make([]uint32, 0, size)
	msg = append(msg, uint32(version)|uint32(size)<<8|uint32(cmd)<<16|REQUEST_TAG<<24)
	msg = append(msg, payload...)

	if err = hw.send(msg); err != nil {
		return
	}

	if rsp, err = hw.receive(); err != nil {
		return
	}

	hdr := rsp[0]

	if tag := hdr >> 24; tag != RESPONSE_TAG {
		return nil, &Error{Reason: "invalid response tag", Value: tag}
	}

	if c := uint8(hdr >> 16); c != cmd {
		return nil, &Error{Reason: "invalid response command", Value: uint32(c)}
	}

	if len(rsp) < 2 {
		return nil, errors.New("missing ELE response code")
	}

	if status := rsp[1] & 0xff; status != STATUS_SUCCESS {
		return nil, &Error{Command: cmd, Reason: "command failed", Value: rsp[1]}
	}

	return rsp[1:], nil
}

// callBuffer executes a command targeting the shared buffer, the payload
// is expected to reference it with the returned address.
func (hw *ELE) callBuffer(size int, fn func(addr uint32) (version uint8, cmd uint8, payload []uint32)) (buf []byte, err error) {
	addr, b := sharedBuffer()

	if size > len(b) {
		return nil, errors.New("invalid ELE buffer size")
	}

	b = b[:size]
	clear(b)

	if hw.Cache != nil {
		// ensure no dirty lines are evicted over ELE writes
		hw.Cache.CleanDataCache(addr, bufSize)
		hw.Cache.InvalidateDataCache(addr, bufSize)
	}

	version, cmd, payload := fn(uint32(addr))

	if _, err = hw.call(version, cmd, payload...); err != nil {
		return
	}

	if hw.Cache != nil {
		hw.Cache.InvalidateDataCache(addr, bufSize)
	}

	return b, nil
}

// GetInfo returns SoC information.
func (hw *ELE) GetInfo() (info *Info, err error) {
	hw.Lock()
	defer hw.Unlock()

	// hdr, soc, lc, uid[4], sha256_rom_patch[8], sha_fw[8], oem_srkh[16],
	// state, oem_pqc_srkh[16], reserved[8]
	const size = 64 * 4

	buf, err := hw.callBuffer(size, func(addr uint32) (uint8, uint8, []uint32) {
		return VERSION_BASELINE, CMD_GET_INFO, []uint32{0, addr, size}
	})

	if err != nil {
		return
	}

	soc := binary.LittleEndian.Uint32(buf[4:8])
	lc := binary.LittleEndian.Uint32(buf[8:12])

	info = &Info{
		SocID:     uint16(soc & 0xffff),
		SocRev:    uint16(soc >> 24),
		Lifecycle: uint16(lc & 0xffff),
	}

	copy(info.UID[:], buf[12:28])

	return
}

// GetEvents returns the ELE event log, which records the errors encountered
// since the last reset (e.g. boot container authentication failures, which
// are not fatal while the lifecycle is not closed).
//
// Each event encodes, from the most significant byte: the command, the
// failure indication, the abort code and the status.
func (hw *ELE) GetEvents() (events []uint32, err error) {
	rsp, err := hw.Call(VERSION_BASELINE, CMD_GET_EVENTS)

	if err != nil {
		return
	}

	if len(rsp) < 2 {
		return nil, errors.New("invalid ELE events response")
	}

	n := int(rsp[1] & 0xffff)

	if n > len(rsp)-2 {
		n = len(rsp) - 2
	}

	return append(events, rsp[2:2+n]...), nil
}

// StartRNG requests the initialization of the ELE TRNG.
func (hw *ELE) StartRNG() (err error) {
	_, err = hw.Call(VERSION_BASELINE, CMD_START_RNG)
	return
}

// TRNGReady returns whether the ELE TRNG is ready to deliver entropy.
func (hw *ELE) TRNGReady() (ready bool, err error) {
	rsp, err := hw.Call(VERSION_BASELINE, CMD_TRNG_STATE)

	if err != nil {
		return
	}

	if len(rsp) < 2 {
		return false, errors.New("invalid ELE TRNG state response")
	}

	return rsp[1]&0xff == TRNG_STATE_READY, nil
}

// GetRandomData fills the argument buffer with random data from the ELE
// TRNG, it panics on error to fit [rng.GetRandomDataFn] requirements.
func (hw *ELE) GetRandomData(b []byte) {
	if err := hw.Read(b); err != nil {
		panic(err)
	}
}

// Read fills the argument buffer with random data from the ELE TRNG.
func (hw *ELE) Read(b []byte) (err error) {
	hw.Lock()
	defer hw.Unlock()

	for len(b) > 0 {
		n := min(len(b), bufSize)

		buf, err := hw.callBuffer(n, func(addr uint32) (uint8, uint8, []uint32) {
			return VERSION_HSM, CMD_RNG_GET, []uint32{RNG_FLAGS_NO_RESEED << 16, addr, uint32(n)}
		})

		if err != nil {
			return err
		}

		copy(b, buf)
		b = b[n:]
	}

	return
}
