// NXP Ethernet Quality of Service (ENET_QOS) driver
// https://github.com/usbarmory/tamago
//
// Copyright (c) The TamaGo Authors. All Rights Reserved.
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

// Package eqos implements a driver for NXP Ethernet Quality of Service
// controllers (ENET_QOS), based on the Synopsys DesignWare Ethernet QoS IP
// (DWC_ether_qos 5.10a), adopting the following reference specifications:
//   - IMX93RM - i.MX 93 Applications Processor Reference Manual - Rev 4 2023/12
//
// This package is only meant to be used with `GOOS=tamago` as supported by the
// TamaGo framework for bare metal Go, see https://github.com/usbarmory/tamago.
package eqos

import (
	"crypto/rand"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/usbarmory/tamago/dma"
	"github.com/usbarmory/tamago/internal/reg"
)

// MAC registers
const (
	MAC_CONFIGURATION = 0x0000
	CONFIGURATION_CST = 21
	CONFIGURATION_ACS = 20
	CONFIGURATION_JE  = 16
	CONFIGURATION_PS  = 15
	CONFIGURATION_FES = 14
	CONFIGURATION_DM  = 13
	CONFIGURATION_LM  = 12
	CONFIGURATION_TE  = 1
	CONFIGURATION_RE  = 0

	MAC_PACKET_FILTER = 0x0008
	PACKET_FILTER_PM  = 4
	PACKET_FILTER_PR  = 0

	MAC_Q0_TX_FLOW_CTRL = 0x0070
	Q0_TX_FLOW_CTRL_PT  = 16
	Q0_TX_FLOW_CTRL_TFE = 1

	MAC_RX_FLOW_CTRL = 0x0090
	RX_FLOW_CTRL_RFE = 0

	MAC_RXQ_CTRL0      = 0x00a0
	RXQ_CTRL0_RXQ0EN   = 0
	RXQ0EN_ENABLED_DCB = 0b10

	MAC_1US_TIC_COUNTER = 0x00dc

	MAC_HW_FEATURE1        = 0x0120
	HW_FEATURE1_TXFIFOSIZE = 6
	HW_FEATURE1_RXFIFOSIZE = 0

	MAC_MDIO_ADDRESS = 0x0200
	MDIO_ADDRESS_PA  = 21
	MDIO_ADDRESS_RDA = 16
	MDIO_ADDRESS_CR  = 8
	MDIO_ADDRESS_GOC = 2
	MDIO_ADDRESS_GB  = 0

	GOC_WRITE = 0b01
	GOC_READ  = 0b11

	MAC_MDIO_DATA = 0x0204

	MAC_ADDRESS0_HIGH = 0x0300
	MAC_ADDRESS0_LOW  = 0x0304
)

// MTL registers
const (
	MTL_TXQ0_OPERATION_MODE   = 0x0d00
	TXQ0_OPERATION_MODE_TQS   = 16
	TXQ0_OPERATION_MODE_TXQEN = 2
	TXQ0_OPERATION_MODE_TSF   = 1
	TXQEN_ENABLED             = 0b10

	MTL_TXQ0_QUANTUM_WEIGHT = 0x0d18

	MTL_RXQ0_OPERATION_MODE  = 0x0d30
	RXQ0_OPERATION_MODE_RQS  = 20
	RXQ0_OPERATION_MODE_RFD  = 14
	RXQ0_OPERATION_MODE_RFA  = 8
	RXQ0_OPERATION_MODE_EHFC = 7
	RXQ0_OPERATION_MODE_RSF  = 5
)

// DMA registers
const (
	DMA_MODE     = 0x1000
	DMA_MODE_SWR = 0

	DMA_SYSBUS_MODE        = 0x1004
	SYSBUS_MODE_RD_OSR_LMT = 16
	SYSBUS_MODE_EAME       = 11
	SYSBUS_MODE_BLEN16     = 3
	SYSBUS_MODE_BLEN8      = 2
	SYSBUS_MODE_BLEN4      = 1

	DMA_CH0_CONTROL   = 0x1100
	CH0_CONTROL_PBLX8 = 16

	DMA_CH0_TX_CONTROL = 0x1104
	TX_CONTROL_TXPBL   = 16
	TX_CONTROL_OSP     = 4
	TX_CONTROL_ST      = 0

	DMA_CH0_RX_CONTROL = 0x1108
	RX_CONTROL_RXPBL   = 16
	RX_CONTROL_RBSZ    = 1
	RX_CONTROL_SR      = 0

	DMA_CH0_TXDESC_LIST_HADDRESS = 0x1110
	DMA_CH0_TXDESC_LIST_ADDRESS  = 0x1114
	DMA_CH0_RXDESC_LIST_HADDRESS = 0x1118
	DMA_CH0_RXDESC_LIST_ADDRESS  = 0x111c
	DMA_CH0_TXDESC_TAIL_POINTER  = 0x1120
	DMA_CH0_RXDESC_TAIL_POINTER  = 0x1128
	DMA_CH0_TXDESC_RING_LENGTH   = 0x112c
	DMA_CH0_RXDESC_RING_LENGTH   = 0x1130

	DMA_CH0_INTERRUPT_ENABLE = 0x1134
	DMA_CH0_STATUS           = 0x1160
)

// DMA channel interrupt events
const (
	IRQ_NIS = 15 // Normal Interrupt Summary
	IRQ_RI  = 6  // Receive Interrupt
	IRQ_TI  = 0  // Transmit Interrupt
)

// MDIO CSR clock ranges (MDC = CSR clock / divider)
var csrClockRanges = []struct {
	max uint32
	cr  uint32
}{
	{35000000, 2},  // 20-35MHz, /16
	{60000000, 3},  // 35-60MHz, /26
	{100000000, 0}, // 60-100MHz, /42
	{150000000, 1}, // 100-150MHz, /62
	{250000000, 4}, // 150-250MHz, /102
	{300000000, 5}, // 250-300MHz, /124
	{500000000, 6}, // 300-500MHz, /204
	{800000000, 7}, // 500-800MHz, /324
}

// Timeout is the default timeout for MDIO and reset operations.
const Timeout = 1 * time.Second

// Stats represents receive error statistics.
type Stats struct {
	// Frames with errors (RDES3 error summary)
	Errors uint32
	// CRC errors
	CRC uint32
	// Receive errors signaled by the PHY (RX_ER)
	Receive uint32
	// Overflow errors
	Overflow uint32
	// Giant (length) errors
	Giant uint32
}

// Cache represents the CPU data cache maintenance operations required when
// frame buffers are allocated in cacheable memory (see [EQOS.Buffers]).
type Cache interface {
	CleanDataCache(addr uint, size int)
	InvalidateDataCache(addr uint, size int)
}

// EQOS represents an Ethernet QoS MAC instance.
type EQOS struct {
	sync.Mutex

	// Controller index
	Index int
	// Base register
	Base uint32
	// CSR (application) clock retrieval function
	Clock func() uint32
	// Interrupt ID (placeholder for caller use)
	IRQ int
	// Clock and interface enable function
	EnableClock func(index int) error
	// Transmit clock configuration function, invoked on speed changes
	SetTXClock func(index int, speed int) error
	// PHY reset function, invoked before the controller software reset,
	// which requires PHY receive clocks to complete.
	ResetPHY func(eth *EQOS) error
	// PHY enable function, invoked after controller initialization
	EnablePHY func(eth *EQOS) error
	// MAC address (use SetMAC() for post Init() changes)
	MAC net.HardwareAddr
	// Descriptor ring size
	RingSize int
	// DMA region for frame data buffers, the global one is used when
	// unset. Descriptors are always allocated in the global region.
	//
	// A dedicated region allows frame buffers to live in cacheable memory
	// while descriptors remain in device (uncached) memory, in which case
	// Cache must be set.
	Buffers *dma.Region
	// Data cache maintenance, required when Buffers is cacheable memory
	Cache Cache
	// Promiscuous mode
	Promiscuous bool

	// Timeout for MDIO and reset operations
	Timeout time.Duration

	// Statistics about received frames discarded due to errors
	Stats Stats

	txFIFOSize uint32
	rxFIFOSize uint32
	cr         uint32

	rx descriptorRing
	tx descriptorRing
}

// Init initializes the Ethernet MAC controller, the link speed is
// configured with [EQOS.SetSpeed] (e.g. within [EQOS.EnablePHY]).
func (hw *EQOS) Init() (err error) {
	hw.Lock()
	defer hw.Unlock()

	if hw.Base == 0 || hw.Clock == nil {
		return errors.New("invalid EQOS controller instance")
	}

	if hw.MAC == nil {
		hw.MAC = make([]byte, 6)
		rand.Read(hw.MAC)
		// flag address as unicast and locally administered
		hw.MAC[0] &= 0xfe
		hw.MAC[0] |= 0x02
	} else if len(hw.MAC) != 6 {
		return errors.New("invalid MAC")
	}

	if hw.RingSize == 0 {
		hw.RingSize = defaultRingSize
	}

	if hw.Timeout == 0 {
		hw.Timeout = Timeout
	}

	if hw.EnableClock != nil {
		if err = hw.EnableClock(hw.Index); err != nil {
			return
		}
	}

	if hw.ResetPHY != nil {
		if err = hw.ResetPHY(hw); err != nil {
			return
		}
	}

	return hw.setup()
}

func (hw *EQOS) setup() (err error) {
	clk := hw.Clock()

	hw.cr = 0xff

	for _, r := range csrClockRanges {
		if clk <= r.max {
			hw.cr = r.cr
			break
		}
	}

	if hw.cr == 0xff {
		return errors.New("unsupported CSR clock")
	}

	// software reset
	reg.Set(hw.Base+DMA_MODE, DMA_MODE_SWR)

	if !reg.WaitFor(hw.Timeout, hw.Base+DMA_MODE, DMA_MODE_SWR, 1, 0) {
		return errors.New("software reset timeout (missing PHY clocks?)")
	}

	// 1us tick counter
	reg.Write(hw.Base+MAC_1US_TIC_COUNTER, clk/1000000-1)

	// FIFO sizes are encoded as log2(n / 128)
	f := reg.Read(hw.Base + MAC_HW_FEATURE1)
	hw.txFIFOSize = 128 << ((f >> HW_FEATURE1_TXFIFOSIZE) & 0x1f)
	hw.rxFIFOSize = 128 << ((f >> HW_FEATURE1_RXFIFOSIZE) & 0x1f)

	tqs := hw.txFIFOSize/256 - 1
	rqs := hw.rxFIFOSize/256 - 1

	// transmit queue: store and forward, enabled, use all FIFO
	txq := hw.Base + MTL_TXQ0_OPERATION_MODE
	reg.Set(txq, TXQ0_OPERATION_MODE_TSF)
	reg.SetN(txq, TXQ0_OPERATION_MODE_TXQEN, 0b11, TXQEN_ENABLED)
	reg.SetN(txq, TXQ0_OPERATION_MODE_TQS, 0x1ff, tqs)
	reg.Write(hw.Base+MTL_TXQ0_QUANTUM_WEIGHT, 0x10)

	// receive queue: store and forward, use all FIFO
	rxq := hw.Base + MTL_RXQ0_OPERATION_MODE
	reg.Set(rxq, RXQ0_OPERATION_MODE_RSF)
	reg.SetN(rxq, RXQ0_OPERATION_MODE_RQS, 0x3ff, rqs)

	// hardware flow control, only with at least 4KB of FIFO
	if rqs >= 4096/256-1 {
		var rfd, rfa uint32

		switch rqs {
		case 4096/256 - 1:
			rfd, rfa = 0x3, 0x1 // Full-3K, Full-1.5K
		case 8192/256 - 1:
			rfd, rfa = 0x6, 0xa // Full-4K, Full-6K
		case 16384/256 - 1:
			rfd, rfa = 0x6, 0x12 // Full-4K, Full-10K
		default:
			rfd, rfa = 0x6, 0x1e // Full-4K, Full-16K
		}

		reg.Set(rxq, RXQ0_OPERATION_MODE_EHFC)
		reg.SetN(rxq, RXQ0_OPERATION_MODE_RFD, 0x3f, rfd)
		reg.SetN(rxq, RXQ0_OPERATION_MODE_RFA, 0x3f, rfa)
	}

	// enable receive queue 0
	reg.SetN(hw.Base+MAC_RXQ_CTRL0, RXQ_CTRL0_RXQ0EN, 0b11, RXQ0EN_ENABLED_DCB)

	// pass all multicast frames (e.g. IPv6 neighbor discovery)
	reg.Set(hw.Base+MAC_PACKET_FILTER, PACKET_FILTER_PM)
	reg.SetTo(hw.Base+MAC_PACKET_FILTER, PACKET_FILTER_PR, hw.Promiscuous)

	// flow control
	reg.SetN(hw.Base+MAC_Q0_TX_FLOW_CTRL, Q0_TX_FLOW_CTRL_PT, 0xffff, 0xffff)
	reg.Set(hw.Base+MAC_Q0_TX_FLOW_CTRL, Q0_TX_FLOW_CTRL_TFE)
	reg.Set(hw.Base+MAC_RX_FLOW_CTRL, RX_FLOW_CTRL_RFE)

	// strip CRC and padding, no jumbo frames, full duplex
	cfg := hw.Base + MAC_CONFIGURATION
	reg.Clear(cfg, CONFIGURATION_JE)
	reg.Clear(cfg, CONFIGURATION_LM)
	reg.Set(cfg, CONFIGURATION_CST)
	reg.Set(cfg, CONFIGURATION_ACS)
	reg.Set(cfg, CONFIGURATION_DM)

	hw.SetMAC(hw.MAC)

	if hw.EnablePHY != nil {
		err = hw.EnablePHY(hw)
	}

	return
}

// SetSpeed configures the MAC for the argument link speed (10, 100 or 1000
// Mbps) and duplex mode, which must match the ones negotiated by the PHY.
func (hw *EQOS) SetSpeed(speed int, fullDuplex bool) (err error) {
	cfg := hw.Base + MAC_CONFIGURATION

	switch speed {
	case 1000:
		reg.Clear(cfg, CONFIGURATION_PS)
		reg.Clear(cfg, CONFIGURATION_FES)
	case 100:
		reg.Set(cfg, CONFIGURATION_PS)
		reg.Set(cfg, CONFIGURATION_FES)
	case 10:
		reg.Set(cfg, CONFIGURATION_PS)
		reg.Clear(cfg, CONFIGURATION_FES)
	default:
		return errors.New("invalid speed")
	}

	reg.SetTo(cfg, CONFIGURATION_DM, fullDuplex)

	if hw.SetTXClock != nil {
		err = hw.SetTXClock(hw.Index, speed)
	}

	return
}

// SetMAC allows to change the controller physical address register after
// initialization.
func (hw *EQOS) SetMAC(mac net.HardwareAddr) {
	hw.MAC = mac

	reg.Write(hw.Base+MAC_ADDRESS0_HIGH, uint32(mac[5])<<8|uint32(mac[4]))
	reg.Write(hw.Base+MAC_ADDRESS0_LOW, uint32(mac[3])<<24|uint32(mac[2])<<16|uint32(mac[1])<<8|uint32(mac[0]))
}

// Start begins processing of incoming packets.
func (hw *EQOS) Start() {
	hw.Lock()
	defer hw.Unlock()

	hw.rx.stats = &hw.Stats
	rxBase := hw.rx.init(true, hw.RingSize, hw.Buffers, hw.Cache)
	txBase := hw.tx.init(false, hw.RingSize, hw.Buffers, hw.Cache)

	// operate on second frame
	reg.Set(hw.Base+DMA_CH0_TX_CONTROL, TX_CONTROL_OSP)
	// receive buffer size
	reg.SetN(hw.Base+DMA_CH0_RX_CONTROL, RX_CONTROL_RBSZ, 0x3fff, bufferSize)
	// contiguous descriptors, 8x burst length multiplier
	reg.Write(hw.Base+DMA_CH0_CONTROL, 1<<CH0_CONTROL_PBLX8)

	// burst length must be < 1/2 FIFO size
	pbl := min(hw.txFIFOSize/256, 32)
	reg.SetN(hw.Base+DMA_CH0_TX_CONTROL, TX_CONTROL_TXPBL, 0x3f, pbl)
	reg.SetN(hw.Base+DMA_CH0_RX_CONTROL, RX_CONTROL_RXPBL, 0x3f, 8)

	reg.Write(hw.Base+DMA_SYSBUS_MODE,
		2<<SYSBUS_MODE_RD_OSR_LMT|
			1<<SYSBUS_MODE_EAME|
			1<<SYSBUS_MODE_BLEN16|
			1<<SYSBUS_MODE_BLEN8|
			1<<SYSBUS_MODE_BLEN4)

	reg.Write(hw.Base+DMA_CH0_TXDESC_LIST_HADDRESS, 0)
	reg.Write(hw.Base+DMA_CH0_TXDESC_LIST_ADDRESS, txBase)
	reg.Write(hw.Base+DMA_CH0_TXDESC_RING_LENGTH, uint32(hw.RingSize-1))

	reg.Write(hw.Base+DMA_CH0_RXDESC_LIST_HADDRESS, 0)
	reg.Write(hw.Base+DMA_CH0_RXDESC_LIST_ADDRESS, rxBase)
	reg.Write(hw.Base+DMA_CH0_RXDESC_RING_LENGTH, uint32(hw.RingSize-1))

	// start DMA and MAC
	reg.Set(hw.Base+DMA_CH0_TX_CONTROL, TX_CONTROL_ST)
	reg.Set(hw.Base+DMA_CH0_RX_CONTROL, RX_CONTROL_SR)
	reg.Set(hw.Base+MAC_CONFIGURATION, CONFIGURATION_TE)
	reg.Set(hw.Base+MAC_CONFIGURATION, CONFIGURATION_RE)

	// all receive descriptors are available
	reg.Write(hw.Base+DMA_CH0_RXDESC_TAIL_POINTER, hw.rx.tail())
}

// EnableInterrupt enables interrupt generation for a specific DMA channel
// event (e.g. [IRQ_RI]).
func (hw *EQOS) EnableInterrupt(event int) {
	reg.Set(hw.Base+DMA_CH0_INTERRUPT_ENABLE, IRQ_NIS)
	reg.Set(hw.Base+DMA_CH0_INTERRUPT_ENABLE, event)
}

// ClearInterrupt clears the interrupt corresponding to a specific DMA channel
// event.
func (hw *EQOS) ClearInterrupt(event int) {
	reg.Write(hw.Base+DMA_CH0_STATUS, 1<<IRQ_NIS|1<<event)
}

func (hw *EQOS) mdioWait() bool {
	return reg.WaitFor(hw.Timeout, hw.Base+MAC_MDIO_ADDRESS, MDIO_ADDRESS_GB, 1, 0)
}

func (hw *EQOS) mdio(op uint32, pa int, ra int) (err error) {
	if !hw.mdioWait() {
		return errors.New("MDIO busy")
	}

	reg.Write(hw.Base+MAC_MDIO_ADDRESS,
		uint32(pa&0x1f)<<MDIO_ADDRESS_PA|
			uint32(ra&0x1f)<<MDIO_ADDRESS_RDA|
			hw.cr<<MDIO_ADDRESS_CR|
			op<<MDIO_ADDRESS_GOC|
			1<<MDIO_ADDRESS_GB)

	if !hw.mdioWait() {
		return errors.New("MDIO timeout")
	}

	return
}

// ReadPHYRegister reads a standard management register of a connected Ethernet
// PHY (IEEE 802.3-2008 Clause 22).
func (hw *EQOS) ReadPHYRegister(pa int, ra int) (data uint16, err error) {
	if err = hw.mdio(GOC_READ, pa, ra); err != nil {
		return
	}

	return uint16(reg.Read(hw.Base + MAC_MDIO_DATA)), nil
}

// WritePHYRegister writes a standard management register of a connected
// Ethernet PHY (IEEE 802.3-2008 Clause 22).
func (hw *EQOS) WritePHYRegister(pa int, ra int, data uint16) (err error) {
	if !hw.mdioWait() {
		return errors.New("MDIO busy")
	}

	reg.Write(hw.Base+MAC_MDIO_DATA, uint32(data))

	return hw.mdio(GOC_WRITE, pa, ra)
}

// Receive receives a single Ethernet frame, excluding the checksum, from the
// MAC controller ring buffer.
func (hw *EQOS) Receive(buf []byte) (n int, err error) {
	n, err = hw.rx.pop(buf)

	if hw.rx.refilled {
		reg.Write(hw.Base+DMA_CH0_RXDESC_TAIL_POINTER, hw.rx.tail())
	}

	return
}

// Transmit transmits a single Ethernet frame, the checksum is appended
// automatically and must not be included.
func (hw *EQOS) Transmit(buf []byte) (err error) {
	if len(buf) > maxFrameSize {
		return errors.New("frame too large")
	}

	if err = hw.tx.push(buf); err != nil {
		return
	}

	reg.Write(hw.Base+DMA_CH0_TXDESC_TAIL_POINTER, hw.tx.tail())

	return
}
