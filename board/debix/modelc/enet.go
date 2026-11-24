// DEBIX Model C support for tamago/arm64
// https://github.com/usbarmory/tamago
//
// Copyright (c) The TamaGo Authors. All Rights Reserved.
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

package modelc

import (
	"errors"
	"fmt"
	"time"

	"github.com/usbarmory/tamago/phy"
	"github.com/usbarmory/tamago/phy/genphy"
	"github.com/usbarmory/tamago/phy/realtek/rtl8211f"
	"github.com/usbarmory/tamago/soc/nxp/enet"
	"github.com/usbarmory/tamago/soc/nxp/eqos"
	"github.com/usbarmory/tamago/soc/nxp/imx93"
	"github.com/usbarmory/tamago/soc/nxp/iomuxc"
)

// On the DEBIX Model C both Ethernet ports are connected in RGMII mode to
// Realtek RTL8211F PHYs (other PHYs are handled through standard registers):
//
//   - ETH2 connector: ENET_QOS, PHY address 1, reset on GPIO3_IO26
//   - ETH1 connector: FEC (i.MX93 ENET1), PHY address 2, reset on GPIO3_IO27
const (
	ENET1_PHY_ADDR       = 2
	ENET1_PHY_RESET_GPIO = 27 // GPIO3_IO27

	ENET_QOS_PHY_ADDR       = 1
	ENET_QOS_PHY_RESET_GPIO = 26 // GPIO3_IO26

	// PHY reset assertion and post-reset delays
	phyResetDuration  = 10 * time.Millisecond
	phyResetPostDelay = 150 * time.Millisecond
)

// IOMUXC registers and pad configurations
// (imx93-pinfunc.h, imx93-debix-base.dts)
const (
	IOMUXC_SW_MUX_CTL_PAD_CCM_CLKO1 = imx93.IOMUXC_BASE + 0x0088
	IOMUXC_SW_PAD_CTL_PAD_CCM_CLKO1 = imx93.IOMUXC_BASE + 0x0238

	IOMUXC_SW_MUX_CTL_PAD_CCM_CLKO2 = imx93.IOMUXC_BASE + 0x008c
	IOMUXC_SW_PAD_CTL_PAD_CCM_CLKO2 = imx93.IOMUXC_BASE + 0x023c

	// ENET1_* (ENET_QOS) and ENET2_* (FEC) pads, in order: MDC, MDIO,
	// TD3, TD2, TD1, TD0, TX_CTL, TXC, RX_CTL, RXC, RD0, RD1, RD2, RD3.
	IOMUXC_SW_MUX_CTL_PAD_ENET1_MDC = imx93.IOMUXC_BASE + 0x0098
	IOMUXC_SW_PAD_CTL_PAD_ENET1_MDC = imx93.IOMUXC_BASE + 0x0248

	IOMUXC_SW_MUX_CTL_PAD_ENET2_MDC = imx93.IOMUXC_BASE + 0x00d0
	IOMUXC_SW_PAD_CTL_PAD_ENET2_MDC = imx93.IOMUXC_BASE + 0x0280

	IOMUX_ALT0_ENET = 0
	IOMUX_ALT5_GPIO = 5

	PAD_CTL_ENET  = 0x57e
	PAD_CTL_CLOCK = 0x5fe
	PAD_CTL_GPIO  = 0x31e
)

// RGMII pad indexes (relative to MDC) carrying clock signals
const (
	padRGMII_TXC = 7
	padRGMII_RXC = 9
	padRGMII_NUM = 14
)

// LinkTimeout is the maximum time waited for the Ethernet link to be
// established during [ENET1] or [ENET_QOS] initialization.
var LinkTimeout = 5 * time.Second

// ErrLinkTimeout is returned on initialization when no link is established
// within [LinkTimeout], the controller is nonetheless initialized (at 100
// Mbps) and can be started.
var ErrLinkTimeout = errors.New("link timeout")

// Ethernet PHYs, set on initialization to an [rtl8211f.PHY] instance or, for
// any other PHY (e.g. under emulation), to a [genphy.PHY] one.
var (
	// ENET1PHY represents the Ethernet PHY connected to [ENET1].
	ENET1PHY phy.PHY

	// ENETQOSPHY represents the Ethernet PHY connected to [ENET_QOS].
	ENETQOSPHY phy.PHY
)

// linkStatus represents the negotiated link status.
type linkStatus struct {
	link       bool
	speed      int
	fullDuplex bool
}

func init() {
	imx93.ENET1.RGMII = true
	imx93.ENET1.EnablePHY = EnablePHY

	imx93.ENET_QOS.ResetPHY = ResetENETQOSPHY
	imx93.ENET_QOS.EnablePHY = EnableENETQOSPHY

	// Default to the addresses assigned to the board (OTP fuses), when
	// not programmed the drivers pick a random one.
	if mac, err := imx93.MAC(1); err == nil {
		imx93.ENET1.MAC = mac
	}

	if mac, err := imx93.MAC(2); err == nil {
		imx93.ENET_QOS.MAC = mac
	}
}

func configureRGMIIPads(mux uint32, ctl uint32) {
	for i := uint32(0); i < padRGMII_NUM; i++ {
		val := uint32(PAD_CTL_ENET)

		if i == padRGMII_TXC || i == padRGMII_RXC {
			val = PAD_CTL_CLOCK
		}

		pad := iomuxc.Init(mux+4*i, ctl+4*i, IOMUX_ALT0_ENET)
		pad.Ctl(val)
	}
}

func resetPHY(mux uint32, ctl uint32, num int) (err error) {
	pad := iomuxc.Init(mux, ctl, IOMUX_ALT5_GPIO)
	pad.Ctl(PAD_CTL_GPIO)

	rst, err := imx93.GPIO3.Init(num)

	if err != nil {
		return
	}

	rst.Out()

	// active low
	rst.Low()
	time.Sleep(phyResetDuration)
	rst.High()
	time.Sleep(phyResetPostDelay)

	return
}

// enablePHY initializes the PHY at the argument address, configuring
// RTL8211F PHYs for `rgmii-id` operation, and waits for link establishment
// to return the negotiated link status.
func enablePHY(addr int, miim phy.MIIM) (p phy.PHY, status linkStatus, err error) {
	gp := &genphy.PHY{}

	if err = gp.Init(addr, miim); err != nil {
		return nil, status, fmt.Errorf("could not initialize PHY, %w", err)
	}

	id, err := gp.Identifier()

	if err != nil {
		return
	}

	var getStatus func() (linkStatus, error)

	if id&rtl8211f.ID_MASK == rtl8211f.ID {
		rp := &rtl8211f.PHY{}

		if err = rp.Init(addr, miim); err != nil {
			return nil, status, fmt.Errorf("could not initialize PHY, %w", err)
		}

		// phy-mode = "rgmii-id"
		if err = rp.SetRGMIIDelay(true, true); err != nil {
			return
		}

		// eee-broken-1000t
		if err = rp.DisableEEE(); err != nil {
			return
		}

		getStatus = func() (linkStatus, error) {
			s, err := rp.Status()
			return linkStatus{s.Link, s.Speed, s.FullDuplex}, err
		}

		p = rp
	} else {
		getStatus = func() (linkStatus, error) {
			s, err := gp.Status()
			return linkStatus{s.Link, s.Speed, s.FullDuplex}, err
		}

		p = gp
	}

	if err = p.Negotiate(); err != nil {
		return
	}

	deadline := time.Now().Add(LinkTimeout)

	for {
		if status, err = getStatus(); err != nil || status.link {
			return
		}

		if time.Now().After(deadline) {
			return p, status, ErrLinkTimeout
		}

		time.Sleep(10 * time.Millisecond)
	}
}

// EnablePHY initializes the Ethernet PHY connected to [ENET1], it waits for
// link establishment (see [LinkTimeout]) to configure the MAC for the
// negotiated speed.
func EnablePHY(eth *enet.ENET) (err error) {
	if eth != imx93.ENET1 {
		return errors.New("invalid ENET instance")
	}

	configureRGMIIPads(IOMUXC_SW_MUX_CTL_PAD_ENET2_MDC, IOMUXC_SW_PAD_CTL_PAD_ENET2_MDC)

	if err = resetPHY(IOMUXC_SW_MUX_CTL_PAD_CCM_CLKO2, IOMUXC_SW_PAD_CTL_PAD_CCM_CLKO2, ENET1_PHY_RESET_GPIO); err != nil {
		return fmt.Errorf("could not reset PHY, %w", err)
	}

	p, status, err := enablePHY(ENET1_PHY_ADDR, eth)
	ENET1PHY = p

	if err != nil {
		return
	}

	return eth.SetSpeed(status.speed)
}

// ResetENETQOSPHY configures the [ENET_QOS] pads and resets its PHY, this
// is required before the controller software reset, which depends on the
// PHY receive clock.
func ResetENETQOSPHY(eth *eqos.EQOS) (err error) {
	if eth != imx93.ENET_QOS {
		return errors.New("invalid ENET_QOS instance")
	}

	configureRGMIIPads(IOMUXC_SW_MUX_CTL_PAD_ENET1_MDC, IOMUXC_SW_PAD_CTL_PAD_ENET1_MDC)

	if err = resetPHY(IOMUXC_SW_MUX_CTL_PAD_CCM_CLKO1, IOMUXC_SW_PAD_CTL_PAD_CCM_CLKO1, ENET_QOS_PHY_RESET_GPIO); err != nil {
		return fmt.Errorf("could not reset PHY, %w", err)
	}

	return
}

// EnableENETQOSPHY initializes the Ethernet PHY connected to [ENET_QOS], it
// waits for link establishment (see [LinkTimeout]) to configure the MAC for
// the negotiated speed.
func EnableENETQOSPHY(eth *eqos.EQOS) (err error) {
	if eth != imx93.ENET_QOS {
		return errors.New("invalid ENET_QOS instance")
	}

	p, status, err := enablePHY(ENET_QOS_PHY_ADDR, eth)
	ENETQOSPHY = p

	if err != nil {
		return
	}

	return eth.SetSpeed(status.speed, status.fullDuplex)
}
