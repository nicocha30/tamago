// DEBIX Model C support for tamago/arm64
// https://github.com/usbarmory/tamago
//
// Copyright (c) The TamaGo Authors. All Rights Reserved.
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

//go:build !linkramsize

package modelc

import (
	_ "unsafe"
)

// Applications can override ramSize with the `linkramsize` build tag.
//
// This is useful on 2GB variants or when large DMA descriptors are required
// to re-initialize tamago `dma` package in external RAM.

// The DEBIX Model C features 1GB LPDDR4X RAM in its default configuration.

//go:linkname ramSize runtime/goos.RamSize
var ramSize uint64 = 0x40000000 // 1GB
