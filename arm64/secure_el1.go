// ARM64 processor support
// https://github.com/usbarmory/tamago
//
// Copyright (c) The TamaGo Authors. All Rights Reserved.
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

//go:build secure_el1

package arm64

// When entered at EL3, cpuinit drops to Secure EL1 (`secure_el1` build tag).
//
// This allows unikernels loaded in place of the EL3 firmware (e.g. as BL31)
// to retain Secure world access to memory and peripherals, including GICv3
// Group 0 interrupts (signaled as FIQ).
//
// This value is read by cpuinit before runtime initialization, therefore it
// must remain a statically initialized variable.
var scrNS uint64 = 0
