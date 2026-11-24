// NXP i.MX93 configuration and support
// https://github.com/usbarmory/tamago
//
// Copyright (c) The TamaGo Authors. All Rights Reserved.
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

package imx93

import (
	"errors"
	"net"

	"github.com/usbarmory/tamago/internal/reg"
)

// On-Chip OTP Controller registers
const (
	// Fuse Shadow Block
	OCOTP_FSB_BASE = 0x47510000
	// OTP shadow registers
	OCOTP_FSB_SHADOW = OCOTP_FSB_BASE + 0x8000

	// MAC address fuse words (MAC1_ADDR, MAC2_ADDR)
	OCOTP_MAC_ADDR0 = 315
	OCOTP_MAC_ADDR1 = 316
	OCOTP_MAC_ADDR2 = 317
)

// ReadFuse returns an OTP fuse word from the Fuse Shadow Block, only words
// exposed by the block (0-51, 312-511) can be read, others require the
// EdgeLock Enclave.
func ReadFuse(word int) (val uint32, err error) {
	if word < 0 || (word > 51 && word < 312) || word > 511 {
		return 0, errors.New("fuse word not available in shadow block")
	}

	return reg.Read(OCOTP_FSB_SHADOW + uint32(word)*4), nil
}

// MAC returns the MAC address programmed in the OTP fuses for the argument
// Ethernet controller index (1: MAC1_ADDR for [ENET1], 2: MAC2_ADDR for
// [ENET_QOS]). An error is returned if the fuses do not hold a valid unicast
// address.
func MAC(index int) (mac net.HardwareAddr, err error) {
	var lo, hi uint32

	mid, _ := ReadFuse(OCOTP_MAC_ADDR1)

	switch index {
	case 1:
		lo, _ = ReadFuse(OCOTP_MAC_ADDR0)
		hi = mid & 0xffff
	case 2:
		lo, _ = ReadFuse(OCOTP_MAC_ADDR2)
		hi = mid >> 16
	default:
		return nil, errors.New("invalid MAC address index")
	}

	mac = net.HardwareAddr{
		byte(hi >> 8), byte(hi),
		byte(lo >> 24), byte(lo >> 16), byte(lo >> 8), byte(lo),
	}

	if (lo == 0 && hi == 0) || (lo == 0xffffffff && hi == 0xffff) || mac[0]&1 != 0 {
		return nil, errors.New("MAC address fuses not programmed")
	}

	return
}
