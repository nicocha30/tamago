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
	CLOCK_ROOT_USDHC1 = 51
	CLOCK_ROOT_USDHC2 = 52
	CLOCK_ROOT_USDHC3 = 53
)

// Low Power Clock Gating registers
const (
	CCM_LPCG_USDHC1 = CCM_LPCG + 0x1380
	CCM_LPCG_USDHC2 = CCM_LPCG + 0x13c0
	CCM_LPCG_USDHC3 = CCM_LPCG + 0x1400
)

// GetUSDHCClock returns the USDHCx_CLK_ROOT frequency.
func GetUSDHCClock(index int) (clock uint32) {
	if index < 1 || index > 3 {
		return
	}

	addr := ClockRoot(CLOCK_ROOT_USDHC1 + index - 1)

	switch reg.GetN(addr, CLOCK_ROOT_MUX, 0b11) {
	case FAST_SEL_OSC_24M:
		clock = OSC_FREQ
	case FAST_SEL_SYS_PLL_PFD0:
		clock = SYS_PLL_PFD0_FREQ
	case FAST_SEL_SYS_PLL_PFD1:
		clock = SYS_PLL_PFD1_FREQ
	case FAST_SEL_SYS_PLL_PFD2:
		clock = SYS_PLL_PFD2_FREQ
	}

	return clock / (reg.GetN(addr, CLOCK_ROOT_DIV, 0xff) + 1)
}

// SetUSDHCClock sets the USDHCx_CLK_ROOT clock and enables its clock gate.
//
// To retain compatibility with the uSDHC driver, which expresses root clock
// frequencies as 396 MHz / (podf + 1), the closest frequency obtainable from
// SYS_PLL_PFD1 (800 MHz) is selected (e.g. podf 1: 200 MHz, podf 2: 133 MHz).
// The clksel argument is ignored.
func SetUSDHCClock(index int, podf uint32, _ uint32) (err error) {
	var lpcg uint32

	switch index {
	case 1:
		lpcg = CCM_LPCG_USDHC1
	case 2:
		lpcg = CCM_LPCG_USDHC2
	case 3:
		lpcg = CCM_LPCG_USDHC3
	default:
		return errors.New("invalid interface index")
	}

	if podf > 7 {
		return errors.New("podf value out of range")
	}

	target := uint32(396000000) / (podf + 1)
	div := (SYS_PLL_PFD1_FREQ + target/2) / target

	// disable clock gate during configuration
	reg.Clear(lpcg, LPCG_ON)
	SetClockRoot(CLOCK_ROOT_USDHC1+index-1, FAST_SEL_SYS_PLL_PFD1, div)
	reg.Set(lpcg, LPCG_ON)

	return
}
