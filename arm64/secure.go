// ARM64 processor support
// https://github.com/usbarmory/tamago
//
// Copyright (c) The TamaGo Authors. All Rights Reserved.
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

//go:build !secure_el1

package arm64

// When entered at EL3, cpuinit drops to Non-secure EL1 (default), the
// `secure_el1` build tag can be used to drop to Secure EL1 instead.
//
// This value is read by cpuinit before runtime initialization, therefore it
// must remain a statically initialized variable.
var scrNS uint64 = 1
