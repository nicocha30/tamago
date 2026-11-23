// DEBIX Model C support for tamago/arm64
// https://github.com/usbarmory/tamago
//
// Copyright (c) The TamaGo Authors. All Rights Reserved.
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

// Package modelc provides hardware initialization, automatically on import,
// for the DEBIX Model C single board computer (NXP i.MX9352).
//
// This package is only meant to be used with `GOOS=tamago GOARCH=arm64` as
// supported by the TamaGo framework for bare metal Go, see
// https://github.com/usbarmory/tamago.
package modelc

import (
	_ "unsafe"

	"github.com/usbarmory/tamago/soc/nxp/imx93"
)

// Peripheral instances
var (
	ENET1    = imx93.ENET1
	ENET_QOS = imx93.ENET_QOS
	LPUART1  = imx93.LPUART1
)

// Init takes care of the lower level initialization triggered early in runtime
// setup (post World start).
//
//go:linkname Init runtime/goos.Hwinit1
func Init() {
	imx93.Init()

	// initialize console
	imx93.LPUART1.Init()
}
