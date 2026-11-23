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

	"github.com/usbarmory/tamago/internal/rng"
)

//go:linkname initRNG runtime/goos.InitRNG
func initRNG() {
	if rng.GetRandomDataFn != nil {
		return
	}

	// The EdgeLock Enclave TRNG is too slow for direct use, therefore we
	// use it to seed an AES-CTR based DRBG.
	ELE.Init()

	// The TRNG is started by the bootloader, wait for it to gather enough
	// entropy (errors are deferred to the actual read attempt).
	for i := 0; i < 1000; i++ {
		if ready, err := ELE.TRNGReady(); err != nil || ready {
			break
		}
	}

	drbg := &rng.DRBG{}

	if err := ELE.Read(drbg.Seed[:]); err != nil {
		panic("imx93: could not seed RNG from EdgeLock Enclave, " + err.Error())
	}

	rng.GetRandomDataFn = drbg.GetRandomData
}
