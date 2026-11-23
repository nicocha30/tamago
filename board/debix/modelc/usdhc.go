// DEBIX Model C support for tamago/arm64
// https://github.com/usbarmory/tamago
//
// Copyright (c) The TamaGo Authors. All Rights Reserved.
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

package modelc

import (
	"time"

	"github.com/usbarmory/tamago/soc/nxp/imx93"
	"github.com/usbarmory/tamago/soc/nxp/iomuxc"
)

// On the DEBIX Model C the optional eMMC is connected to uSDHC1 (8-bit) and
// the microSD slot to uSDHC2 (4-bit), with its power switch enabled by
// GPIO3_IO07 and its card detection on GPIO3_IO00.

// MMC represents the eMMC card interface (uSDHC1).
var MMC = imx93.USDHC1

// SD represents the microSD card interface (uSDHC2).
var SD = imx93.USDHC2

const (
	MMC_BUS_WIDTH = 8
	SD_BUS_WIDTH  = 4

	SD_POWER_GPIO = 7 // GPIO3_IO07
	SD_CD_GPIO    = 0 // GPIO3_IO00

	// SD power switch off-on delay
	sdPowerDelay = 12 * time.Millisecond
)

// IOMUXC registers and pad configurations
// (imx93-pinfunc.h, imx93-debix-base.dts)
const (
	// SD1_* pads, in order: CLK, CMD, DATA0-7, STROBE
	IOMUXC_SW_MUX_CTL_PAD_SD1_CLK = imx93.IOMUXC_BASE + 0x0108
	IOMUXC_SW_PAD_CTL_PAD_SD1_CLK = imx93.IOMUXC_BASE + 0x02b8

	// SD2_* pads, in order: CLK, CMD, DATA0-3
	IOMUXC_SW_MUX_CTL_PAD_SD2_CLK = imx93.IOMUXC_BASE + 0x0154
	IOMUXC_SW_PAD_CTL_PAD_SD2_CLK = imx93.IOMUXC_BASE + 0x0304

	IOMUXC_SW_MUX_CTL_PAD_SD2_VSELECT = imx93.IOMUXC_BASE + 0x0134
	IOMUXC_SW_PAD_CTL_PAD_SD2_VSELECT = imx93.IOMUXC_BASE + 0x02e4

	IOMUXC_SW_MUX_CTL_PAD_SD2_CD_B = imx93.IOMUXC_BASE + 0x0150
	IOMUXC_SW_PAD_CTL_PAD_SD2_CD_B = imx93.IOMUXC_BASE + 0x0300

	IOMUXC_SW_MUX_CTL_PAD_SD2_RESET_B = imx93.IOMUXC_BASE + 0x016c
	IOMUXC_SW_PAD_CTL_PAD_SD2_RESET_B = imx93.IOMUXC_BASE + 0x031c

	IOMUX_ALT0_USDHC = 0

	PAD_CTL_SD_CLOCK   = 0x15fe
	PAD_CTL_SD_DATA    = 0x13fe
	PAD_CTL_SD_VSELECT = 0x51e
)

func configureSDPads(mux uint32, ctl uint32, num int, clocks ...int) {
	for i := 0; i < num; i++ {
		val := uint32(PAD_CTL_SD_DATA)

		for _, c := range clocks {
			if i == c {
				val = PAD_CTL_SD_CLOCK
			}
		}

		pad := iomuxc.Init(mux+uint32(4*i), ctl+uint32(4*i), IOMUX_ALT0_USDHC)
		pad.Ctl(val)
	}
}

// InitMMC configures the eMMC pads and initializes its controller, the card
// is detected with [MMC.Detect].
func InitMMC() {
	// CLK, CMD, DATA0-7, STROBE (clock signals: CLK, STROBE)
	configureSDPads(IOMUXC_SW_MUX_CTL_PAD_SD1_CLK, IOMUXC_SW_PAD_CTL_PAD_SD1_CLK, 11, 0, 10)

	MMC.Init(MMC_BUS_WIDTH)
}

// InitSD configures the microSD pads, enables card power and initializes
// its controller, the card is detected with [SD.Detect].
func InitSD() (err error) {
	// CLK, CMD, DATA0-3 (clock signals: CLK)
	configureSDPads(IOMUXC_SW_MUX_CTL_PAD_SD2_CLK, IOMUXC_SW_PAD_CTL_PAD_SD2_CLK, 6, 0)

	vsel := iomuxc.Init(IOMUXC_SW_MUX_CTL_PAD_SD2_VSELECT, IOMUXC_SW_PAD_CTL_PAD_SD2_VSELECT, IOMUX_ALT0_USDHC)
	vsel.Ctl(PAD_CTL_SD_VSELECT)

	cd := iomuxc.Init(IOMUXC_SW_MUX_CTL_PAD_SD2_CD_B, IOMUXC_SW_PAD_CTL_PAD_SD2_CD_B, IOMUX_ALT5_GPIO)
	cd.Ctl(PAD_CTL_GPIO)

	pwr := iomuxc.Init(IOMUXC_SW_MUX_CTL_PAD_SD2_RESET_B, IOMUXC_SW_PAD_CTL_PAD_SD2_RESET_B, IOMUX_ALT5_GPIO)
	pwr.Ctl(PAD_CTL_GPIO)

	power, err := imx93.GPIO3.Init(SD_POWER_GPIO)

	if err != nil {
		return
	}

	// power cycle the card (VSD_3V3)
	power.Out()
	power.Low()
	time.Sleep(sdPowerDelay)
	power.High()
	time.Sleep(sdPowerDelay)

	SD.Init(SD_BUS_WIDTH)

	return
}

// SDCardPresent returns whether a microSD card is inserted.
func SDCardPresent() bool {
	cd, err := imx93.GPIO3.Init(SD_CD_GPIO)

	if err != nil {
		return false
	}

	cd.In()

	// active low
	return !cd.Value()
}
