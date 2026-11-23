// NXP Rapid General Purpose Input/Output (RGPIO) driver
// https://github.com/usbarmory/tamago
//
// Copyright (c) The TamaGo Authors. All Rights Reserved.
//
// Use of this source code is governed by the license
// that can be found in the LICENSE file.

// Package rgpio implements helpers for GPIO configuration on NXP SoCs
// featuring RGPIO controllers (e.g. i.MX 8ULP, i.MX 9x) adopting the
// following reference specifications:
//   - IMX93RM - i.MX 93 Applications Processor Reference Manual - Rev 4 2023/12
//
// This package is only meant to be used with `GOOS=tamago` as supported by the
// TamaGo framework for bare metal Go, see https://github.com/usbarmory/tamago.
package rgpio

import (
	"errors"

	"github.com/usbarmory/tamago/internal/reg"
)

// RGPIO registers
const (
	GPIO_PDOR = 0x40 // Port Data Output
	GPIO_PSOR = 0x44 // Port Set Output
	GPIO_PCOR = 0x48 // Port Clear Output
	GPIO_PTOR = 0x4c // Port Toggle Output
	GPIO_PDIR = 0x50 // Port Data Input
	GPIO_PDDR = 0x54 // Port Data Direction
)

// GPIO controller instance
type GPIO struct {
	// Controller index
	Index int
	// Base register
	Base uint32
}

// Pin instance
type Pin struct {
	num  int
	pdor uint32
	psor uint32
	pcor uint32
	pdir uint32
	pddr uint32
}

// Init initializes a GPIO.
func (hw *GPIO) Init(num int) (gpio *Pin, err error) {
	if hw.Base == 0 {
		return nil, errors.New("invalid GPIO controller instance")
	}

	if num < 0 || num > 31 {
		return nil, errors.New("invalid GPIO number")
	}

	gpio = &Pin{
		num:  num,
		pdor: hw.Base + GPIO_PDOR,
		psor: hw.Base + GPIO_PSOR,
		pcor: hw.Base + GPIO_PCOR,
		pdir: hw.Base + GPIO_PDIR,
		pddr: hw.Base + GPIO_PDDR,
	}

	return
}

// Out configures a GPIO as output.
func (gpio *Pin) Out() {
	reg.Set(gpio.pddr, gpio.num)
}

// In configures a GPIO as input.
func (gpio *Pin) In() {
	reg.Clear(gpio.pddr, gpio.num)
}

// High configures a GPIO signal as high.
func (gpio *Pin) High() {
	reg.Write(gpio.psor, 1<<gpio.num)
}

// Low configures a GPIO signal as low.
func (gpio *Pin) Low() {
	reg.Write(gpio.pcor, 1<<gpio.num)
}

// Value returns the GPIO signal level.
func (gpio *Pin) Value() (high bool) {
	return reg.Get(gpio.pdir, gpio.num)
}
