// RTL8211F Ethernet PHY support
// https://github.com/usbarmory/tamago
//
// Copyright (c) The TamaGo Authors. All Rights Reserved.
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

// Package rtl8211f implements a driver for the Realtek RTL8211F family of
// Gigabit Ethernet Transceivers (e.g. RTL8211F-CG, RTL8211FD-VX) adopting the
// following specifications:
//   - RTL8211F(I)-CG / RTL8211FD(I)-CG Datasheet - Rev 1.4
package rtl8211f

import (
	"errors"
	"fmt"
	"time"

	"github.com/usbarmory/tamago/bits"
	"github.com/usbarmory/tamago/phy"
)

// PHY identifier (revision bits excluded)
const (
	ID      = 0x001cc910
	ID_MASK = 0xfffffff0
)

// Standard registers
const (
	BASIC_CONTROL = 0x00
	BASIC_STATUS  = 0x01
	PHY_ID_1      = 0x02
	PHY_ID_2      = 0x03
	MMD_CTRL      = 0x0d
	MMD_DATA      = 0x0e

	CTRL_RESET        = 15
	CTRL_ANEG_ENABLE  = 12
	CTRL_ANEG_RESTART = 9

	STATUS_ANEG_COMPLETE = 5
	STATUS_LINK          = 2

	MMD_CTRL_DATA = 0x4000
)

// EEE registers (MMD 7)
const (
	MMD_AN     = 7
	MMD_AN_EEE = 0x3c
)

// Paged registers
const (
	PAGE_SELECT = 0x1f

	// PHY Specific Status Register
	PAGE_PHYSR   = 0xa43
	PHYSR        = 0x1a
	PHYSR_SPEED  = 4
	PHYSR_DUPLEX = 3
	PHYSR_LINK   = 2

	// RGMII delay configuration
	PAGE_MIICR      = 0xd08
	MIICR1          = 0x11
	MIICR1_TX_DELAY = 8
	MIICR2          = 0x15
	MIICR2_RX_DELAY = 3
)

// Timeout is the default timeout for PHY operations.
const Timeout = 100 * time.Millisecond

// Status represents the PHY link status.
type Status struct {
	Link       bool
	Speed      int
	FullDuplex bool
}

// PHY represents an RTL8211F PHY instance.
type PHY struct {
	// Timeout for PHY operations
	Timeout time.Duration

	miim  phy.MIIM
	pa    int
	speed int
}

// Init initializes the PHY instance, verifying its identifier.
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

	if id&ID_MASK != ID {
		return fmt.Errorf("unsupported PHY identifier (%#x)", id)
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

// readPage reads a paged register, restoring the standard page afterwards.
func (hw *PHY) readPage(page uint16, address int) (data uint16, err error) {
	if err = hw.write(PAGE_SELECT, page); err != nil {
		return
	}

	data, err = hw.read(address)

	if e := hw.write(PAGE_SELECT, 0); err == nil {
		err = e
	}

	return
}

// modifyPage sets or clears a paged register bit, restoring the standard
// page afterwards.
func (hw *PHY) modifyPage(page uint16, address int, pos int, val bool) (err error) {
	var data uint16

	if err = hw.write(PAGE_SELECT, page); err != nil {
		return
	}

	defer func() {
		if e := hw.write(PAGE_SELECT, 0); err == nil {
			err = e
		}
	}()

	if data, err = hw.read(address); err != nil {
		return
	}

	bits.SetTo16(&data, pos, val)

	return hw.write(address, data)
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

// SetRGMIIDelay enables or disables the PHY internal RGMII transmit and
// receive clock delays (e.g. both enabled for `rgmii-id` configurations).
func (hw *PHY) SetRGMIIDelay(tx bool, rx bool) (err error) {
	if err = hw.modifyPage(PAGE_MIICR, MIICR1, MIICR1_TX_DELAY, tx); err != nil {
		return
	}

	return hw.modifyPage(PAGE_MIICR, MIICR2, MIICR2_RX_DELAY, rx)
}

// DisableEEE disables Energy Efficient Ethernet advertisement, it must be
// followed by [PHY.Negotiate] to take effect.
func (hw *PHY) DisableEEE() (err error) {
	if err = hw.write(MMD_CTRL, MMD_AN); err != nil {
		return
	}

	if err = hw.write(MMD_DATA, MMD_AN_EEE); err != nil {
		return
	}

	if err = hw.write(MMD_CTRL, MMD_CTRL_DATA|MMD_AN); err != nil {
		return
	}

	return hw.write(MMD_DATA, 0)
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
	var basic, physr uint16

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

	if physr, err = hw.readPage(PAGE_PHYSR, PHYSR); err != nil {
		return
	}

	status.Link = bits.Get16(&physr, PHYSR_LINK)
	status.FullDuplex = bits.Get16(&physr, PHYSR_DUPLEX)

	switch bits.GetN16(&physr, PHYSR_SPEED, 0b11) {
	case 0b00:
		status.Speed = 10
	case 0b01:
		status.Speed = 100
	case 0b10:
		status.Speed = 1000
	default:
		return status, errors.New("invalid PHY speed")
	}

	if status.Link {
		hw.speed = status.Speed
	}

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
