// NXP i.MX93 configuration and support
// https://github.com/usbarmory/tamago
//
// Copyright (c) The TamaGo Authors. All Rights Reserved.
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

package imx93

import (
	"github.com/usbarmory/tamago/internal/reg"
)

// Clock Control Module registers
const (
	CCM_BASE = 0x44450000

	// Clock roots
	CCM_CLOCK_ROOT = CCM_BASE + 0x0000
	CLOCK_ROOT_OFF = 24
	CLOCK_ROOT_MUX = 8
	CLOCK_ROOT_DIV = 0

	// Low Power Clock Gating
	CCM_LPCG = CCM_BASE + 0x8000
	LPCG_ON  = 0
)

// Clock root indexes
const (
	CLOCK_ROOT_LPUART1 = 25
	CLOCK_ROOT_LPUART2 = 26
	CLOCK_ROOT_LPUART3 = 27
	CLOCK_ROOT_LPUART4 = 28
	CLOCK_ROOT_LPUART5 = 29
	CLOCK_ROOT_LPUART6 = 30
	CLOCK_ROOT_LPUART7 = 31
	CLOCK_ROOT_LPUART8 = 32
)

// Low Power Clock Gating registers
const (
	CCM_LPCG_LPUART1 = CCM_LPCG + 0x0d00
	CCM_LPCG_LPUART2 = CCM_LPCG + 0x0d40
	CCM_LPCG_LPUART3 = CCM_LPCG + 0x0d80
	CCM_LPCG_LPUART4 = CCM_LPCG + 0x0dc0
	CCM_LPCG_LPUART5 = CCM_LPCG + 0x0e00
	CCM_LPCG_LPUART6 = CCM_LPCG + 0x0e40
	CCM_LPCG_LPUART7 = CCM_LPCG + 0x0e80
	CCM_LPCG_LPUART8 = CCM_LPCG + 0x0ec0
)

// Clock root sources for low speed I/O peripherals (e.g. LPUART)
const (
	LOW_SPEED_IO_SEL_OSC_24M           = 0
	LOW_SPEED_IO_SEL_SYS_PLL_PFD0_DIV2 = 1
	LOW_SPEED_IO_SEL_SYS_PLL_PFD1_DIV2 = 2
	LOW_SPEED_IO_SEL_VIDEO_PLL         = 3
)

// Reference clocks
const (
	OSC_FREQ = 24000000 // 24MHz
)

// ClockRoot returns the address of a clock root control register.
func ClockRoot(index int) uint32 {
	return CCM_CLOCK_ROOT + uint32(index)*0x80
}

// SetClockRoot configures a clock root source (mux) and divider (1-256).
func SetClockRoot(index int, mux uint32, div uint32) {
	if div == 0 || div > 256 {
		panic("invalid clock root divider")
	}

	addr := ClockRoot(index)

	reg.SetN(addr, CLOCK_ROOT_MUX, 0b11, mux)
	reg.SetN(addr, CLOCK_ROOT_DIV, 0xff, div-1)
	reg.Clear(addr, CLOCK_ROOT_OFF)
}

// GetUARTClock returns the LPUART clock root frequency.
//
// The LPUART1 clock root is configured by the bootloader (U-Boot) to use the
// 24MHz oscillator without division, other instances must be configured with
// [InitUARTClock] before use.
func GetUARTClock() uint32 {
	return OSC_FREQ
}

// InitUARTClock sets the clock root of the argument LPUART instance index
// (1-8) to the 24MHz oscillator.
func InitUARTClock(index int) {
	if index < 1 || index > 8 {
		panic("invalid LPUART index")
	}

	SetClockRoot(CLOCK_ROOT_LPUART1+index-1, LOW_SPEED_IO_SEL_OSC_24M, 1)
}
