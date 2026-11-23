// Generic IEEE 802.3 Clause 22 Ethernet PHY support
// https://github.com/usbarmory/tamago
//
// Copyright (c) The TamaGo Authors. All Rights Reserved.
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

// Package genphy implements a driver for Ethernet PHYs using only the
// standard management registers defined in IEEE 802.3-2008 Clause 22.
//
// It is meant as fallback for PHYs without a dedicated driver (e.g. emulated
// ones).
package genphy

import (
	"errors"
	"time"

	"github.com/usbarmory/tamago/bits"
	"github.com/usbarmory/tamago/phy"
)

// Standard registers
const (
	BASIC_CONTROL   = 0x00
	BASIC_STATUS    = 0x01
	PHY_ID_1        = 0x02
	PHY_ID_2        = 0x03
	PHY_ANEG_ADV    = 0x04
	PHY_ANEG_LPA    = 0x05
	PHY_1000_CTRL   = 0x09
	PHY_1000_STATUS = 0x0a

	CTRL_RESET        = 15
	CTRL_ANEG_ENABLE  = 12
	CTRL_ANEG_RESTART = 9

	STATUS_EXTENDED      = 8
	STATUS_ANEG_COMPLETE = 5
	STATUS_LINK          = 2

	ANEG_ADV_1000_FULL = 9
	ANEG_ADV_1000_HALF = 8
	ANEG_LPA_1000_FULL = 11
	ANEG_LPA_1000_HALF = 10

	ANEG_100_FULL = 8
	ANEG_100_HALF = 7
	ANEG_10_FULL  = 6
	ANEG_10_HALF  = 5
)

// Timeout is the default timeout for PHY operations.
const Timeout = 100 * time.Millisecond

// Status represents the PHY link status.
type Status struct {
	Link       bool
	Speed      int
	FullDuplex bool
}

// PHY represents a generic Clause 22 PHY instance.
type PHY struct {
	// Timeout for PHY operations
	Timeout time.Duration

	miim  phy.MIIM
	pa    int
	speed int
}

// Init initializes the PHY instance.
func (hw *PHY) Init(addr int, miim phy.MIIM) (err error) {
	if miim == nil || addr < 0 || addr > 31 {
		return errors.New("invalid PHY instance")
	}

	hw.miim = miim
	hw.pa = addr
	hw.speed = 0

	if hw.Timeout == 0 {
		hw.Timeout = Timeout
	}

	id, err := hw.Identifier()

	if err != nil {
		return
	}

	if id == 0 || id == 0xffffffff {
		return errors.New("no PHY detected")
	}

	return
}

func (hw *PHY) read(address int) (data uint16, err error) {
	if hw.miim == nil {
		return 0, errors.New("invalid PHY instance")
	}

	return hw.miim.ReadPHYRegister(hw.pa, address)
}

func (hw *PHY) write(address int, data uint16) (err error) {
	if hw.miim == nil {
		return errors.New("invalid PHY instance")
	}

	return hw.miim.WritePHYRegister(hw.pa, address, data)
}

// Identifier returns the PHY identifier.
func (hw *PHY) Identifier() (id uint32, err error) {
	var high, low uint16

	if high, err = hw.read(PHY_ID_1); err != nil {
		return
	}

	if low, err = hw.read(PHY_ID_2); err != nil {
		return
	}

	return uint32(high)<<16 | uint32(low), nil
}

// Reset performs a PHY software reset.
func (hw *PHY) Reset() (err error) {
	var control uint16

	if err = hw.write(BASIC_CONTROL, 1<<CTRL_RESET); err != nil {
		return
	}

	deadline := time.Now().Add(hw.Timeout)

	for {
		if control, err = hw.read(BASIC_CONTROL); err != nil {
			return
		}

		if !bits.Get16(&control, CTRL_RESET) {
			return
		}

		if time.Now().After(deadline) {
			return errors.New("reset timeout")
		}

		time.Sleep(time.Millisecond)
	}
}

// Negotiate enables and restarts auto-negotiation.
func (hw *PHY) Negotiate() (err error) {
	var control uint16

	hw.speed = 0

	if control, err = hw.read(BASIC_CONTROL); err != nil {
		return
	}

	control |= (1 << CTRL_ANEG_ENABLE) | (1 << CTRL_ANEG_RESTART)

	return hw.write(BASIC_CONTROL, control)
}

// Status returns the PHY link status, as resolved by auto-negotiation.
func (hw *PHY) Status() (status Status, err error) {
	var basic, local, partner uint16

	hw.speed = 0

	// STATUS_LINK is latch-low; the first read clears stale link state.
	if _, err = hw.read(BASIC_STATUS); err != nil {
		return
	}

	if basic, err = hw.read(BASIC_STATUS); err != nil {
		return
	}

	if !bits.Get16(&basic, STATUS_LINK) || !bits.Get16(&basic, STATUS_ANEG_COMPLETE) {
		return
	}

	if bits.Get16(&basic, STATUS_EXTENDED) {
		if local, err = hw.read(PHY_1000_CTRL); err != nil {
			return
		}

		if partner, err = hw.read(PHY_1000_STATUS); err != nil {
			return
		}

		switch {
		case bits.Get16(&local, ANEG_ADV_1000_FULL) && bits.Get16(&partner, ANEG_LPA_1000_FULL):
			status.Speed = 1000
			status.FullDuplex = true
		case bits.Get16(&local, ANEG_ADV_1000_HALF) && bits.Get16(&partner, ANEG_LPA_1000_HALF):
			status.Speed = 1000
		}
	}

	if status.Speed == 0 {
		if local, err = hw.read(PHY_ANEG_ADV); err != nil {
			return
		}

		if partner, err = hw.read(PHY_ANEG_LPA); err != nil {
			return
		}

		common := local & partner

		switch {
		case bits.Get16(&common, ANEG_100_FULL):
			status.Speed = 100
			status.FullDuplex = true
		case bits.Get16(&common, ANEG_100_HALF):
			status.Speed = 100
		case bits.Get16(&common, ANEG_10_FULL):
			status.Speed = 10
			status.FullDuplex = true
		case bits.Get16(&common, ANEG_10_HALF):
			status.Speed = 10
		default:
			return status, errors.New("PHY has no common advertised mode")
		}
	}

	status.Link = true
	hw.speed = status.Speed

	return
}

// Address returns the PHY address passed at [PHY.Init].
func (hw *PHY) Address() int {
	return hw.pa
}

// Speed returns the last resolved PHY link speed.
func (hw *PHY) Speed() int {
	return hw.speed
}
