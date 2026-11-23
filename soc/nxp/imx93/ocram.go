// NXP i.MX93 OCRAM support
// https://github.com/usbarmory/tamago
//
// Copyright (c) The TamaGo Authors. All Rights Reserved.
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

package imx93

// On-Chip Random-Access Memory
//
// The i.MX93 features 640KB of OCRAM (0x20480000-0x2051ffff), its upper
// 256KB are reserved as the Arm Trusted Firmware (BL31) runtime region,
// therefore only the lower 384KB are used.
const (
	OCRAM_START = 0x20480000
	OCRAM_SIZE  = 0x60000
)
