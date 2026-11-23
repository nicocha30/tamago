// NXP i.MX93 configuration and support
// https://github.com/usbarmory/tamago
//
// Copyright (c) The TamaGo Authors. All Rights Reserved.
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

// Package imx93 provides support to Go bare metal unikernels, written using
// the TamaGo framework, on the NXP i.MX 93 family of System-on-Chip (SoC)
// application processors.
//
// The package implements initialization and drivers for NXP i.MX93 SoCs,
// adopting the following reference specifications:
//   - IMX93RM - i.MX 93 Applications Processor Reference Manual - Rev 4 2023/12
//
// This package is only meant to be used with `GOOS=tamago GOARCH=arm64` as
// supported by the TamaGo framework for bare metal Go, see
// https://github.com/usbarmory/tamago.
package imx93

import (
	"fmt"

	"github.com/usbarmory/tamago/arm64"
	"github.com/usbarmory/tamago/arm64/gic"

	"github.com/usbarmory/tamago/soc/nxp/ele"
	"github.com/usbarmory/tamago/soc/nxp/enet"
	"github.com/usbarmory/tamago/soc/nxp/eqos"
	"github.com/usbarmory/tamago/soc/nxp/lpuart"
	"github.com/usbarmory/tamago/soc/nxp/rgpio"
	"github.com/usbarmory/tamago/soc/nxp/usb"
	"github.com/usbarmory/tamago/soc/nxp/usdhc"
)

// i.MX93 SoC identifier, as reported by the EdgeLock Enclave
const IMX93 = 0x9300

// Interrupts
const (
	// The first 32 interrupts are private to the CPUs' interface.
	BASE_IRQ = 32

	// EdgeLock Enclave Messaging Unit
	ELE_MU_IRQ = BASE_IRQ + 31

	// Ethernet MAC (FEC)
	ENET1_IRQ = BASE_IRQ + 179

	// Ethernet Quality of Service MAC
	ENET_QOS_IRQ = BASE_IRQ + 184

	// Serial ports
	LPUART1_IRQ = BASE_IRQ + 19
	LPUART2_IRQ = BASE_IRQ + 20
	LPUART3_IRQ = BASE_IRQ + 68
	LPUART4_IRQ = BASE_IRQ + 69
	LPUART5_IRQ = BASE_IRQ + 70
	LPUART6_IRQ = BASE_IRQ + 71
	LPUART7_IRQ = BASE_IRQ + 210
	LPUART8_IRQ = BASE_IRQ + 211

	// USB controllers
	USB1_IRQ = BASE_IRQ + 187
	USB2_IRQ = BASE_IRQ + 188

	// SD/MMC controllers
	USDHC1_IRQ = BASE_IRQ + 86
	USDHC2_IRQ = BASE_IRQ + 87
	USDHC3_IRQ = BASE_IRQ + 205
)

// Peripheral registers
const (
	// DDR base address
	DDR_BASE = 0x80000000

	// EdgeLock Enclave Messaging Unit (non-secure)
	ELE_MU_BASE = 0x47520000

	// Ethernet MAC (FEC)
	ENET1_BASE = 0x42890000

	// Ethernet Quality of Service MAC
	ENET_QOS_BASE = 0x428a0000

	// General Interrupt Controller
	GICD_BASE = 0x48000000
	GICR_BASE = 0x48040000

	// General Purpose I/O
	GPIO1_BASE = 0x47400000
	GPIO2_BASE = 0x43810000
	GPIO3_BASE = 0x43820000
	GPIO4_BASE = 0x43830000

	// I/O multiplexer controller
	IOMUXC_BASE = 0x443c0000

	// Serial ports
	LPUART1_BASE = 0x44380000
	LPUART2_BASE = 0x44390000
	LPUART3_BASE = 0x42570000
	LPUART4_BASE = 0x42580000
	LPUART5_BASE = 0x42590000
	LPUART6_BASE = 0x425a0000
	LPUART7_BASE = 0x42690000
	LPUART8_BASE = 0x426a0000

	// USB controllers
	USB1_BASE = 0x4c100000
	USB2_BASE = 0x4c200000

	// SD/MMC controllers
	USDHC1_BASE = 0x42850000
	USDHC2_BASE = 0x42860000
	USDHC3_BASE = 0x428b0000
)

// Peripheral instances
var (
	// ARM64 core
	ARM64 = &arm64.CPU{
		// required before Init()
		TimerOffset: 1,
	}

	// EdgeLock Enclave
	ELE = &ele.ELE{
		Base:  ELE_MU_BASE,
		Cache: ARM64,
	}

	// Ethernet MAC 1 (FEC)
	ENET1 = &enet.ENET{
		Index:     1,
		Base:      ENET1_BASE,
		Clock:     GetENETClock,
		EnablePLL: EnableENETClock,
		IRQ:       ENET1_IRQ,
	}

	// Ethernet Quality of Service MAC
	ENET_QOS = &eqos.EQOS{
		Index:       1,
		Base:        ENET_QOS_BASE,
		Clock:       GetENETClock,
		EnableClock: EnableENETQOSClock,
		IRQ:         ENET_QOS_IRQ,
	}

	// Generic Interrupt Controller
	GIC = &gic.GIC{
		GICD: GICD_BASE,
		GICR: GICR_BASE,
	}

	// GPIO controller 1
	GPIO1 = &rgpio.GPIO{
		Index: 1,
		Base:  GPIO1_BASE,
	}

	// GPIO controller 2
	GPIO2 = &rgpio.GPIO{
		Index: 2,
		Base:  GPIO2_BASE,
	}

	// GPIO controller 3
	GPIO3 = &rgpio.GPIO{
		Index: 3,
		Base:  GPIO3_BASE,
	}

	// GPIO controller 4
	GPIO4 = &rgpio.GPIO{
		Index: 4,
		Base:  GPIO4_BASE,
	}

	// Serial port 1
	LPUART1 = &lpuart.LPUART{
		Index: 1,
		Base:  LPUART1_BASE,
		LPCG:  CCM_LPCG_LPUART1,
		Clock: GetUARTClock,
		IRQ:   LPUART1_IRQ,
	}

	// Serial port 2
	LPUART2 = &lpuart.LPUART{
		Index: 2,
		Base:  LPUART2_BASE,
		LPCG:  CCM_LPCG_LPUART2,
		Clock: GetUARTClock,
		IRQ:   LPUART2_IRQ,
	}

	// Serial port 3
	LPUART3 = &lpuart.LPUART{
		Index: 3,
		Base:  LPUART3_BASE,
		LPCG:  CCM_LPCG_LPUART3,
		Clock: GetUARTClock,
		IRQ:   LPUART3_IRQ,
	}

	// Serial port 4
	LPUART4 = &lpuart.LPUART{
		Index: 4,
		Base:  LPUART4_BASE,
		LPCG:  CCM_LPCG_LPUART4,
		Clock: GetUARTClock,
		IRQ:   LPUART4_IRQ,
	}

	// Serial port 5
	LPUART5 = &lpuart.LPUART{
		Index: 5,
		Base:  LPUART5_BASE,
		LPCG:  CCM_LPCG_LPUART5,
		Clock: GetUARTClock,
		IRQ:   LPUART5_IRQ,
	}

	// Serial port 6
	LPUART6 = &lpuart.LPUART{
		Index: 6,
		Base:  LPUART6_BASE,
		LPCG:  CCM_LPCG_LPUART6,
		Clock: GetUARTClock,
		IRQ:   LPUART6_IRQ,
	}

	// Serial port 7
	LPUART7 = &lpuart.LPUART{
		Index: 7,
		Base:  LPUART7_BASE,
		LPCG:  CCM_LPCG_LPUART7,
		Clock: GetUARTClock,
		IRQ:   LPUART7_IRQ,
	}

	// Serial port 8
	LPUART8 = &lpuart.LPUART{
		Index: 8,
		Base:  LPUART8_BASE,
		LPCG:  CCM_LPCG_LPUART8,
		Clock: GetUARTClock,
		IRQ:   LPUART8_IRQ,
	}

	// USB controller 1
	USB1 = &usb.USB{
		Index:     1,
		Base:      USB1_BASE,
		IRQ:       USB1_IRQ,
		EnablePLL: EnableUSBClock,
	}

	// USB controller 2
	USB2 = &usb.USB{
		Index:     2,
		Base:      USB2_BASE,
		IRQ:       USB2_IRQ,
		EnablePLL: EnableUSBClock,
	}

	// SD/MMC controller 1
	USDHC1 = &usdhc.USDHC{
		Index:    1,
		Base:     USDHC1_BASE,
		SetClock: SetUSDHCClock,
	}

	// SD/MMC controller 2
	USDHC2 = &usdhc.USDHC{
		Index:    2,
		Base:     USDHC2_BASE,
		SetClock: SetUSDHCClock,
	}

	// SD/MMC controller 3
	USDHC3 = &usdhc.USDHC{
		Index:    3,
		Base:     USDHC3_BASE,
		SetClock: SetUSDHCClock,
	}
)

// SiliconVersion returns the SoC identifier and silicon revision (e.g. 0xa1
// for revision A1) as reported by the EdgeLock Enclave.
func SiliconVersion() (socID uint16, socRev uint16, err error) {
	info, err := ELE.GetInfo()

	if err != nil {
		return
	}

	return info.SocID, info.SocRev, nil
}

// UniqueID returns the NXP SoC Device Unique 128-bit ID, as reported by the
// EdgeLock Enclave.
func UniqueID() (uid [16]byte, err error) {
	info, err := ELE.GetInfo()

	if err != nil {
		return
	}

	return info.UID, nil
}

// Model returns the SoC model name.
func Model() (model string) {
	id, rev, err := SiliconVersion()

	switch {
	case err != nil:
		model = "unknown"
	case id == IMX93:
		model = "i.MX 93"
	default:
		return fmt.Sprintf("unknown (%#x)", id)
	}

	if err == nil && rev>>4 >= 0xa {
		model += fmt.Sprintf(" rev %c%d", 'A'+rune(rev>>4-0xa), rev&0xf)
	}

	return
}
