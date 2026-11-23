// DEBIX Model C support for tamago/arm64
// https://github.com/usbarmory/tamago
//
// Copyright (c) The TamaGo Authors. All Rights Reserved.
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

package modelc

import (
	"github.com/usbarmory/tamago/soc/nxp/imx93"
	"github.com/usbarmory/tamago/soc/nxp/iomuxc"
)

// On the DEBIX Model C the USB 2.0 OTG Type-C connector, which also provides
// board power, is connected to the i.MX93 USB1 controller. Its VBUS output
// (host role) is switched by GPIO2_IO10, which must be kept off in device
// role, when VBUS is supplied by the connected host.

// OTG represents the USB OTG Type-C controller (USB1).
var OTG = imx93.USB1

// IOMUXC registers
const (
	IOMUXC_SW_MUX_CTL_PAD_GPIO_IO10 = imx93.IOMUXC_BASE + 0x0038
	IOMUXC_SW_PAD_CTL_PAD_GPIO_IO10 = imx93.IOMUXC_BASE + 0x01e8

	// GPIO_IO10 pad: GPIO2_IO10 function
	IOMUX_ALT0_GPIO2_IO10 = 0

	USB1_VBUS_GPIO = 10 // GPIO2_IO10
)

// InitUSB disables the OTG connector VBUS output and initializes its
// controller, which can then be set in device mode (see [usb.USB.DeviceMode]).
func InitUSB() (err error) {
	pad := iomuxc.Init(IOMUXC_SW_MUX_CTL_PAD_GPIO_IO10, IOMUXC_SW_PAD_CTL_PAD_GPIO_IO10, IOMUX_ALT0_GPIO2_IO10)
	pad.Ctl(PAD_CTL_GPIO)

	vbus, err := imx93.GPIO2.Init(USB1_VBUS_GPIO)

	if err != nil {
		return
	}

	// device role: never drive VBUS towards the host
	vbus.Low()
	vbus.Out()

	OTG.Init()

	return
}

// USBVBUSPresent returns whether VBUS is supplied on the OTG connector (e.g.
// a host is connected).
func USBVBUSPresent() bool {
	return imx93.USBVBUSValid(OTG.Index)
}
