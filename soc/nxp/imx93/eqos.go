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
	CLOCK_ROOT_ENET        = 86
	CLOCK_ROOT_ENET_TIMER2 = 88
)

// Low Power Clock Gating registers
const (
	CCM_LPCG_ENET_QOS = CCM_LPCG + 0x1e40
)

// WAKEUPMIX block control registers
const (
	BLK_CTRL_WAKEUPMIX_BASE = 0x42420000

	WAKEUPMIX_EQOS_GPR  = BLK_CTRL_WAKEUPMIX_BASE + 0x28
	EQOS_GPR_INTF_SEL   = 1
	EQOS_GPR_CLK_GEN_EN = 0
	EQOS_INTF_SEL_MII   = 0b000
	EQOS_INTF_SEL_RGMII = 0b001
	EQOS_INTF_SEL_RMII  = 0b100
)

// SYS_PLL_PFD0_DIV2 frequency
const sysPLLPFD0Div2 = SYS_PLL_PFD0_FREQ / 2

// EnableENETQOSClock configures the ENET_QOS clock roots and RGMII interface
// selection, and enables its clock gate.
func EnableENETQOSClock(index int) error {
	if index != 1 {
		return errors.New("invalid ENET_QOS index")
	}

	// disable clock gate during configuration
	reg.Clear(CCM_LPCG_ENET_QOS, LPCG_ON)

	// select RGMII and enable RGMII transmit clock generation
	reg.SetN(WAKEUPMIX_EQOS_GPR, EQOS_GPR_INTF_SEL, 0b111, EQOS_INTF_SEL_RGMII)
	reg.Set(WAKEUPMIX_EQOS_GPR, EQOS_GPR_CLK_GEN_EN)

	// 250MHz transmit root clock, the MAC derives the RGMII transmit
	// clock (125MHz, 25MHz or 2.5MHz) according to the link speed.
	SetClockRoot(CLOCK_ROOT_ENET, CLOCK_SEL_SYS_PLL_PFD0_DIV2, sysPLLPFD0Div2/250000000)
	// 100MHz IEEE 1588 timer clock
	SetClockRoot(CLOCK_ROOT_ENET_TIMER2, CLOCK_SEL_SYS_PLL_PFD0_DIV2, 5)

	reg.Set(CCM_LPCG_ENET_QOS, LPCG_ON)

	return nil
}
