// NXP i.MX93 configuration and support
// https://github.com/usbarmory/tamago
//
// Copyright (c) The TamaGo Authors. All Rights Reserved.
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

package imx93

import (
	"errors"

	"github.com/usbarmory/tamago/internal/reg"
)

// Clock root indexes
const (
	CLOCK_ROOT_HSIO    = 61
	CLOCK_ROOT_USB_PHY = 93
)

// Low Power Clock Gating registers
const (
	CCM_LPCG_USB_CONTROLLER = CCM_LPCG + 0x1a00
	CCM_LPCG_HSIO_32K       = CCM_LPCG + 0x1dc0
)

// USB non-core registers (usbmisc), relative to the controller base
const (
	USBNC_OFFSET = 0x200

	USBNC_USB_CTRL        = 0x00
	USB_CTRL_OVER_CUR_DIS = 7
	USB_CTRL_NON_BURST    = 1

	USBNC_USB_CTRL2              = 0x04
	USB_CTRL2_AUTO_RESUME        = 2
	USB_CTRL2_VBUS_WAKEUP_SOURCE = 0
	VBUS_WAKEUP_SOURCE_BVALID    = 0b10

	USBNC_USB_OTG_PHY_CFG2 = 0x34
	PHY_CFG2_DRVVBUS0      = 16

	USBNC_USB_OTG_PHY_STATUS = 0x3c
	PHY_STATUS_VBUS_VLD      = 3
)

// EnableUSBClock enables the USB controller clocks and configures its
// integrated PHY (i.MX93 USB controllers do not require a dedicated PLL).
func EnableUSBClock(index int) error {
	var base uint32

	switch index {
	case 1:
		base = USB1_BASE
	case 2:
		base = USB2_BASE
	default:
		return errors.New("invalid USB index")
	}

	// 133MHz HSIO root clock (SYS_PLL_PFD1_DIV2 / 3)
	SetClockRoot(CLOCK_ROOT_HSIO, LOW_SPEED_IO_SEL_SYS_PLL_PFD1_DIV2, 3)
	// USB PHY clock root (as set by the boot ROM)
	reg.Clear(ClockRoot(CLOCK_ROOT_USB_PHY), CLOCK_ROOT_OFF)

	// controllers clock gate (shared) and wakeup clock
	reg.Set(CCM_LPCG_USB_CONTROLLER, LPCG_ON)
	reg.Set(CCM_LPCG_HSIO_32K, LPCG_ON)

	nc := base + USBNC_OFFSET

	// disable over-current detection, set non-burst AHB transfers
	reg.Set(nc+USBNC_USB_CTRL, USB_CTRL_OVER_CUR_DIS)
	reg.Set(nc+USBNC_USB_CTRL, USB_CTRL_NON_BURST)

	// use B-session valid as VBUS wakeup source
	reg.SetN(nc+USBNC_USB_CTRL2, USB_CTRL2_VBUS_WAKEUP_SOURCE, 0b11, VBUS_WAKEUP_SOURCE_BVALID)
	reg.Set(nc+USBNC_USB_CTRL2, USB_CTRL2_AUTO_RESUME)

	// power the PHY VBUS valid comparator (required for device mode
	// connection detection)
	reg.Set(nc+USBNC_USB_OTG_PHY_CFG2, PHY_CFG2_DRVVBUS0)

	return nil
}

// USBVBUSValid returns whether VBUS is detected on the argument USB
// controller index PHY.
func USBVBUSValid(index int) bool {
	switch index {
	case 1:
		return reg.Get(USB1_BASE+USBNC_OFFSET+USBNC_USB_OTG_PHY_STATUS, PHY_STATUS_VBUS_VLD)
	case 2:
		return reg.Get(USB2_BASE+USBNC_OFFSET+USBNC_USB_OTG_PHY_STATUS, PHY_STATUS_VBUS_VLD)
	}

	return false
}
