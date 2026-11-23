TamaGo - bare metal Go - DEBIX Model C support
==============================================

tamago | https://github.com/usbarmory/tamago  

Copyright (c) The TamaGo Authors. All Rights Reserved.  

![TamaGo gopher](https://github.com/usbarmory/tamago/wiki/images/tamago.svg?sanitize=true)

Authors
=======

Andrea Barisani  
andrea@inversepath.com  

Andrej Rosano  
andrej@inversepath.com  

Introduction
============

TamaGo is a framework that enables compilation and execution of unencumbered Go
applications on bare metal processors.

The [modelc](https://github.com/usbarmory/tamago/tree/master/board/debix/modelc)
package provides support for the [DEBIX Model C](https://debix.io/hardware/model-c.html)
single board computer, based on the NXP i.MX9352 SoC.

Documentation
=============

[![Go Reference](https://pkg.go.dev/badge/github.com/usbarmory/tamago.svg)](https://pkg.go.dev/github.com/usbarmory/tamago)

For more information about TamaGo see its
[repository](https://github.com/usbarmory/tamago) and
[project wiki](https://github.com/usbarmory/tamago/wiki).

For the underlying driver support for this board see package
[imx93](https://github.com/usbarmory/tamago/tree/master/soc/nxp/imx93).

The package API documentation can be found on
[pkg.go.dev](https://pkg.go.dev/github.com/usbarmory/tamago).

Supported hardware
==================

| SoC          | Board                                                       | SoC package                                                            | Board package                                                                      |
|--------------|-------------------------------------------------------------|------------------------------------------------------------------------|------------------------------------------------------------------------------------|
| NXP i.MX9352 | [DEBIX Model C](https://debix.io/hardware/model-c.html)     | [imx93](https://github.com/usbarmory/tamago/tree/master/soc/nxp/imx93) | [debix/modelc](https://github.com/usbarmory/tamago/tree/master/board/debix/modelc) |

> [!WARNING]
> This package is in early development stages.

Compiling
=========

Go distribution supporting `GOOS=tamago`
---------------------------------------

The [tamago](https://github.com/usbarmory/tamago/tree/latest/cmd/tamago)
command downloads, compiles, and runs the `go` command from the
[TamaGo distribution](https://github.com/usbarmory/tamago-go) matching the
tamago module version from the application `go.mod`.

Applications can add `github.com/usbarmory/tamago` to `go.mod`, and then
replace the `go` command with:


```sh
go run github.com/usbarmory/tamago/cmd/tamago
```

or add the following line to `go.mod` to use `go tool tamago` as go command:

```
tool github.com/usbarmory/tamago/cmd/tamago
```

Alternatively the
[latest TamaGo distribution](https://github.com/usbarmory/tamago-go/tree/latest) can be
manually built or the
[latest binary release](https://github.com/usbarmory/tamago-go/releases/latest) can be used:

```sh
wget https://github.com/usbarmory/tamago-go/archive/refs/tags/latest.zip
unzip latest.zip
cd tamago-go-latest/src && ./all.bash
cd ../bin && export TAMAGO=`pwd`/go
```

Building applications
---------------------

Go applications are required to set `GOOSPKG` to the desired
[runtime/goos](https://github.com/usbarmory/tamago-go/tree/latest/src/runtime/goos)
overlay and import the relevant board package to ensure that hardware
initialization and runtime support take place:

```golang
import (
	_ "github.com/usbarmory/tamago/board/debix/modelc"
)
```

Go applications can be compiled as usual, using the compiler built in the
previous step, but with the addition of the following flags/variables:

```sh
GOOS=tamago GOOSPKG=github.com/usbarmory/tamago GOARCH=arm64 \
	${TAMAGO} build -ldflags "-T 0x80010000 -R 0x1000" main.go
```

The [go-net](https://github.com/usbarmory/go-net) package provides drivers for
TCP/IP connectivity.

Ethernet
========

Both Ethernet ports are supported, each connected in RGMII mode to a Realtek
RTL8211F PHY:

| Connector  | Controller                | Board variable    | PHY address | PHY reset  |
|------------|---------------------------|-------------------|-------------|------------|
| `ETH1`     | ENET_QOS (`soc/nxp/eqos`) | `modelc.ENET_QOS` | 1           | GPIO3_IO26 |
| `ETH2`     | FEC (`soc/nxp/enet`)      | `modelc.ENET1`    | 2           | GPIO3_IO27 |

Their initialization configures pads, clocks and the PHY, then waits for
link establishment (`modelc.LinkTimeout`, 5 seconds by default) to set the
MAC speed (10/100/1000 Mbps). Without link `modelc.ErrLinkTimeout` is
returned, the controller is nonetheless initialized at 100 Mbps and can be
started.

```golang
if err := modelc.ENET1.Init(); err != nil {
	log.Fatal(err)
}

modelc.ENET1.Start()

if err := modelc.ENET_QOS.Init(); err != nil {
	log.Fatal(err)
}

modelc.ENET_QOS.Start()
```

Both controllers expose the same `Receive`/`Transmit` frame interface, the
[go-net](https://github.com/usbarmory/go-net) package can be used for TCP/IP
connectivity. Without an explicit `MAC` each interface uses a random locally
administered address.

USB
===

The USB 2.0 OTG Type-C connector is connected to the i.MX93 USB1 controller
(`modelc.OTG`), which can be used in device mode with the
[usb](https://github.com/usbarmory/tamago/tree/master/soc/nxp/usb) driver
(e.g. CDC-ECM Ethernet, mass storage):

```golang
if err := modelc.InitUSB(); err != nil {
	log.Fatal(err)
}

modelc.OTG.DeviceMode()
go modelc.OTG.Start(device)
```

`modelc.InitUSB()` keeps the connector VBUS output (GPIO2_IO10) disabled, as
VBUS is supplied by the connected host in device mode, `modelc.USBVBUSPresent()`
reports its presence.

USB host mode is not supported.

Storage
=======

| Interface        | Controller | Board variable | Initialization     | Notes                                       |
|------------------|------------|----------------|--------------------|---------------------------------------------|
| eMMC (optional)  | uSDHC1     | `modelc.MMC`   | `modelc.InitMMC()` | 8-bit                                       |
| microSD          | uSDHC2     | `modelc.SD`    | `modelc.InitSD()`  | 4-bit, power on GPIO3_IO07, CD on GPIO3_IO00 |

Cards are detected with `Detect()` and accessed with `ReadBlocks()` and
`WriteBlocks()`. The eMMC hardware boot partitions are accessed with
`ReadPartitionBlocks()` and `WritePartitionBlocks()`, which preserve the
eMMC boot configuration (e.g. boot0 holding U-Boot) and select the user area
again on completion:

```golang
modelc.InitMMC()

if err := modelc.MMC.Detect(); err != nil {
	log.Fatal(err)
}

// second boot partition (mmcblk0boot1 on Linux)
err := modelc.MMC.WritePartitionBlocks(usdhc.PARTITION_ACCESS_BOOT2, 0, buf)
```

Build tags
==========

The following build tags allow applications to override the package own
definition for the `runtime/goos` overlay:

* `linkramsize`: exclude `ramSize` from `mem.go` (e.g. for 2GB variants)
* `linkprintk`: exclude `printk` from `console.go`

Executing and debugging
=======================

The DEBIX Model C boot chain (ROM, EdgeLock Enclave firmware, U-Boot SPL,
Arm Trusted Firmware and U-Boot) is left unchanged, TamaGo unikernels are
loaded by U-Boot and run in non-secure mode.

The serial console is LPUART1 (115200 8N1), available on the 40-pin header
(3.3V levels).

U-Boot
------

U-Boot starts the unikernel in Non-secure EL2, TamaGo then drops to
Non-secure EL1. The DEBIX U-Boot is configured with `bootdelay=0`, holding a
key on the serial console during power-on stops autoboot.

The ELF image can be loaded and executed from the U-Boot prompt, as an
example from the microSD card first partition:

```
fatload mmc 1:1 0x90000000 main
bootelf 0x90000000
```

Alternatively the image can be loaded over the network:

```
dhcp 0x90000000 main
bootelf 0x90000000
```

The unikernel can also be started with `booti`, once wrapped in an arm64
Linux `Image` header branching to its entry point (linking at `0x80011000`
leaves room for the header at `0x80010000`):

```
fatload mmc 1:1 0x90000000 Image
booti 0x90000000 - ${fdtcontroladdr}
```

The ELF load address must not overlap with the image segments, which start
at the `-T` linker address (`0x80010000`).

The unikernel uses all RAM from `0x80000000` up to `ramSize` (1GB), when the
board firmware includes OP-TEE (`tee.bin`, loaded by default by NXP BSPs at
`0x96000000`) the `linkramsize` build tag must be used to define a `ramSize`
which excludes the secure world memory.

Secure mode without U-Boot
--------------------------

TamaGo unikernels can be entered directly at EL3, in place of TF-A and U-Boot
proper, and remain in the Secure world by compiling with the `secure_el1`
build tag (see [arm64](https://github.com/usbarmory/tamago/tree/master/arm64)):

```sh
GOOS=tamago GOOSPKG=github.com/usbarmory/tamago GOARCH=arm64 \
	${TAMAGO} build -tags secure_el1 -ldflags "-T 0x80010000 -R 0x1000" main.go
```

In this configuration no other software runs on the Cortex-A55 after the
U-Boot SPL, which remains required for DDR initialization: the SPL loads the
unikernel in DDR from the second boot container (in place of TF-A and U-Boot)
and branches to it at EL3. With AHAB secure boot enabled, both containers are
authenticated by the EdgeLock Enclave.

> [!WARNING]
> This mode has only been tested under emulation (QEMU), as the unikernel
> image is not yet packaged in a boot container.

QEMU
----

The board package can be executed under emulation with the
[qemu-imx93](https://github.com/kylefoxaustin/qemu-imx93) QEMU fork, which
models the i.MX 93 11x11 EVK (same SoC, LPUART1 console and Ethernet PHY
addresses as the DEBIX Model C):

```sh
qemu-system-aarch64 \
	-machine imx93-11x11-evk -m 1G \
	-nographic -monitor none -serial stdio \
	-nic tap,ifname=tap0,script=no,downscript=no \
	-nic tap,ifname=tap1,script=no,downscript=no \
	-kernel main
```

The first `-nic` is connected to `ENET1` (FEC), the second one to `ENET_QOS`.

SD cards and eMMC devices can be attached as follows (emulated eMMC devices are
connected to uSDHC3, rather than uSDHC1, by QEMU):

```sh
	-drive if=sd,bus=1,file=sd.img,format=raw \
	-drive if=none,id=emmc0,file=emmc.img,format=raw \
	-device emmc,drive=emmc0,bus=sd-bus,boot-partition-size=1048576,boot-config=8
```
Connecting both to the same hub (`-nic hubport,hubid=0` twice) allows to test
frame exchange between the two controllers without host networking.

Emulated PHYs are not RTL8211F parts and are therefore handled through
standard registers at 100 Mbps. The emulated ENET_QOS reports received frames
including their 4 bytes checksum, unlike real hardware.

License
=======

tamago | https://github.com/usbarmory/tamago  
Copyright (c) The TamaGo Authors. All Rights Reserved.

These source files are distributed under the BSD-style license found in the
[LICENSE](https://github.com/usbarmory/tamago/blob/master/LICENSE) file.

The TamaGo logo is adapted from the Go gopher designed by Renee French and
licensed under the Creative Commons 3.0 Attributions license. Go Gopher vector
illustration by Hugo Arganda.
