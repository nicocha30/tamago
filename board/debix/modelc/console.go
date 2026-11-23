// DEBIX Model C support for tamago/arm64
// https://github.com/usbarmory/tamago
//
// Copyright (c) The TamaGo Authors. All Rights Reserved.
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

//go:build !linkprintk

package modelc

import (
	_ "unsafe"

	"github.com/usbarmory/tamago/soc/nxp/imx93"
)

// On the DEBIX Model C the debug serial console is LPUART1, available on the
// 40-pin header (3.3V), therefore standard output is redirected there.

//go:linkname printk runtime/goos.Printk
func printk(c byte) {
	if c == '\n' {
		imx93.LPUART1.Tx('\r')
	}

	imx93.LPUART1.Tx(c)
}
