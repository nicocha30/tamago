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
	CLOCK_ROOT_WAKEUP_AXI  = 7
	CLOCK_ROOT_ENET_TIMER1 = 87
	CLOCK_ROOT_ENET_REF    = 89
)

// Low Power Clock Gating registers
const (
	CCM_LPCG_ENET1 = CCM_LPCG + 0x1e00
)

// Clock root sources
const (
	// LOW_SPEED_IO_SEL, NON_IO_SEL
	CLOCK_SEL_SYS_PLL_PFD0_DIV2 = 1

	// FAST_SEL
	FAST_SEL_OSC_24M      = 0
	FAST_SEL_SYS_PLL_PFD0 = 1
	FAST_SEL_SYS_PLL_PFD1 = 2
	FAST_SEL_SYS_PLL_PFD2 = 3
)

// System PLL fractional dividers outputs, as configured by the boot ROM
const (
	SYS_PLL_PFD0_FREQ = 1000000000 // 1GHz
	SYS_PLL_PFD1_FREQ = 800000000  // 800MHz
	SYS_PLL_PFD2_FREQ = 625000000  // 625MHz
)

// GetENETClock returns the ENET (FEC) IPG and ENET_QOS CSR clock frequency,
// derived from the WAKEUP_AXI clock root.
func GetENETClock() uint32 {
	var parent uint32

	addr := ClockRoot(CLOCK_ROOT_WAKEUP_AXI)

	switch reg.GetN(addr, CLOCK_ROOT_MUX, 0b11) {
	case FAST_SEL_OSC_24M:
		parent = OSC_FREQ
	case FAST_SEL_SYS_PLL_PFD0:
		parent = SYS_PLL_PFD0_FREQ
	case FAST_SEL_SYS_PLL_PFD1:
		parent = SYS_PLL_PFD1_FREQ
	case FAST_SEL_SYS_PLL_PFD2:
		parent = SYS_PLL_PFD2_FREQ
	}

	return parent / (reg.GetN(addr, CLOCK_ROOT_DIV, 0xff) + 1)
}

// EnableENETClock configures the ENET (FEC) clock roots for RGMII operation
// and enables its clock gate.
func EnableENETClock(index int) error {
	if index != 1 {
		return errors.New("invalid ENET index")
	}

	// disable clock gate during configuration
	reg.Clear(CCM_LPCG_ENET1, LPCG_ON)

	// 250MHz reference clock (125MHz RGMII transmit clock)
	SetClockRoot(CLOCK_ROOT_ENET_REF, CLOCK_SEL_SYS_PLL_PFD0_DIV2, 2)
	// 100MHz IEEE 1588 timer clock
	SetClockRoot(CLOCK_ROOT_ENET_TIMER1, CLOCK_SEL_SYS_PLL_PFD0_DIV2, 5)

	reg.Set(CCM_LPCG_ENET1, LPCG_ON)

	return nil
}
