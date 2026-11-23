// NXP i.MX93 configuration and support
// https://github.com/usbarmory/tamago
//
// Copyright (c) The TamaGo Authors. All Rights Reserved.
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

package imx93

import (
	_ "unsafe"
)

// System Counter control frame
const SYS_CNT_BASE = 0x44290000

func initTimers() {
	// The system counter (24MHz) is configured and started by the
	// secure firmware, use its frequency as set in CNTFRQ_EL0.
	ARM64.InitGenericTimers(SYS_CNT_BASE, 0)
}

//go:linkname nanotime runtime/goos.Nanotime
func nanotime() int64 {
	return ARM64.GetTime()
}
