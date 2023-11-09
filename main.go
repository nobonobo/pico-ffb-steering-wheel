package main

import (
	"bufio"
	"log"
	"machine"
	"machine/usb/hid/joystick"
	"os"
	"strconv"
	"strings"
	"time"

	"tinygo.org/x/drivers/mcp2515"

	"diy-ffb-wheel/motor"
	"diy-ffb-wheel/pid"
	"diy-ffb-wheel/utils"
)

const (
	LED1      machine.Pin = 25
	LED2      machine.Pin = 14
	LED3      machine.Pin = 15
	SW1       machine.Pin = 24
	SW2       machine.Pin = 23
	SW3       machine.Pin = 22
	CAN_INT   machine.Pin = 16
	CAN_RESET machine.Pin = 17
	CAN_SCK   machine.Pin = 18
	CAN_TX    machine.Pin = 19
	CAN_RX    machine.Pin = 20
	CAN_CS    machine.Pin = 21

	// Device Specific Configuration
	NeutralAdjust       = -6.5 // unit:deg
	Lock2Lock           = 1080 // unit:deg
	CoggingTorqueCancel = 128  // 32768 // unit:100*n/256 %
	Viscosity           = 128  // 30000 // unit:100*n/256 %
	CenteringForce      = 500  // unit:100*n/32767 %
	SoftLockForce       = 8    // unit:100*n %

	HalfLock2Lock = Lock2Lock / 2
	MaxAngle      = 32768*HalfLock2Lock/360 - 1
)

var (
	spi       = machine.SPI0
	js        = joystick.Port()
	ph        *pid.PIDHandler
	dummyMode = false
)

func init() {
	ph = pid.NewPIDHandler()
	js = joystick.UseSettings(joystick.Definitions{
		ReportID:     1,
		ButtonCnt:    24,
		HatSwitchCnt: 0,
		AxisDefs: []joystick.Constraint{
			{MinIn: -32767, MaxIn: 32767, MinOut: -32767, MaxOut: 32767},
			{MinIn: 0, MaxIn: 32767, MinOut: 0, MaxOut: 32767},
			{MinIn: 0, MaxIn: 32767, MinOut: 0, MaxOut: 32767},
			{MinIn: 0, MaxIn: 32767, MinOut: 0, MaxOut: 32767},
			{MinIn: 0, MaxIn: 32767, MinOut: 0, MaxOut: 32767},
			{MinIn: -32767, MaxIn: 32767, MinOut: -32767, MaxOut: 32767},
		},
	}, ph.RxHandler, ph.SetupHandler, pid.Descriptor)
	LED1.Configure(machine.PinConfig{Mode: machine.PinOutput})
	LED2.Configure(machine.PinConfig{Mode: machine.PinOutput})
	LED3.Configure(machine.PinConfig{Mode: machine.PinOutput})
	LED1.High()
	LED2.High()
	LED3.High()
	SW1.Configure(machine.PinConfig{Mode: machine.PinInput})
	SW2.Configure(machine.PinConfig{Mode: machine.PinInput})
	SW3.Configure(machine.PinConfig{Mode: machine.PinInput})
	CAN_INT.Configure(machine.PinConfig{Mode: machine.PinInput})
	CAN_RESET.Configure(machine.PinConfig{Mode: machine.PinOutput})
	CAN_RESET.Low()
	time.Sleep(10 * time.Millisecond)
	CAN_RESET.High()
	time.Sleep(10 * time.Millisecond)
}

var (
	axMap = map[int]int{
		2: 1, // side
		3: 2, // throttle
		4: 4, // brake
		5: 3, // clutch
		9: 0, // steering
	}
	shift = [][]int{
		0: {2, 0, 1},
		1: {4, 0, 3},
		2: {6, 0, 5},
		3: {8, 0, 7},
	}
	fitx   = utils.Map(-32767, 32767, 0, 4)
	limitx = utils.Limit(0, 3)
	fity   = utils.Map(-32767, 32767, 0, 3)
	limity = utils.Limit(0, 2)
)

func setShift(x, y int32) (neutral bool) {
	const begin = 10
	dx, dy := limitx(fitx(x)), limity(fity(y))
	s := shift[dx][dy]
	for i := 1; i < 9; i++ {
		if i == s {
			js.SetButton(i+begin-1, true)
		} else {
			js.SetButton(i+begin-1, false)
		}
	}
	return s == 0
}

func absInt32(n int32) int32 {
	if n < 0 {
		return -n
	}
	return n
}

func pow3(v int32) int32 {
	r := v * v / 256
	r = r * v / 256
	return r
}

func main() {
	LED1.Low()
	log.SetFlags(log.Lmicroseconds)
	if err := spi.Configure(
		machine.SPIConfig{
			Frequency: 500000,
			SCK:       CAN_SCK,
			SDO:       CAN_TX,
			SDI:       CAN_RX,
			Mode:      0,
		},
	); err != nil {
		log.Print(err)
	}
	go func() {
		time.Sleep(1 * time.Second)
		axises := make([]int32, 11)
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			dummyMode = false
			for i, s := range strings.Split(scanner.Text(), ",") {
				if i >= len(axises) {
					break
				}
				v, err := strconv.Atoi(s)
				if err != nil {
					break
				}
				axises[i] = int32(v)
				dummyMode = i >= 9
			}
			for i, v := range axises {
				idx, ok := axMap[i]
				if ok {
					js.SetAxis(idx, int(v))
					if idx == 0 {
						js.SetAxis(0, int(v))
						js.SetAxis(5, int(v))
					}
				}
				if dummyMode && i == 10 {
					js.Buttons[0] = byte((v >> 0) & 0xff)
					js.Buttons[1] = byte((v >> 8) & 0xff)
					js.Buttons[2] = byte((v >> 16) & 0xff)
				}
			}
			if !dummyMode {
				neutral := setShift(axises[0], axises[1])
				// for sequential mode
				switch {
				case axises[7] > 0:
					js.SetButton(8, true)
				case axises[6] > 0:
					js.SetButton(9, true)
				default:
					js.SetButton(8, false)
					js.SetButton(9, false)
				}
				if neutral {
					js.SetButton(0, axises[3] > 8192)
					js.SetButton(1, axises[4] > 8192)
					js.SetButton(2, axises[5] > 8192)
					js.SetButton(3, axises[2] > 8192)
				} else {
					js.SetButton(0, false)
					js.SetButton(1, false)
					js.SetButton(2, false)
					js.SetButton(3, false)
				}
			}
		}
		log.Print(scanner.Err())
	}()
	can := mcp2515.New(spi, CAN_CS)
	can.Configure()
	if err := can.Begin(mcp2515.CAN500kBps, mcp2515.Clock8MHz); err != nil {
		log.Fatal(err)
	}
	if err := motor.Setup(can); err != nil {
		log.Fatal(err)
	}
	motor.SetNeutralAdjust(NeutralAdjust)
	ticker := time.NewTicker(1 * time.Millisecond)
	fit := utils.Map(-MaxAngle, MaxAngle, -32767, 32767)
	limit1 := utils.Limit(-32767, 32767)
	limit2 := utils.Limit(-CenteringForce, CenteringForce)
	cnt := 0
	for range ticker.C {
		state, err := motor.GetState(can)
		if err != nil {
			log.Print(err)
		}
		verocity := 256 * int32(state.Verocity) / 220
		angle := fit(state.Angle)
		output := limit2(-angle)              // Centering
		cog := CoggingTorqueCancel * verocity // Cogging Torque Cancel
		decel := -Viscosity * pow3(verocity)  // Viscosity
		output += int32(cog + decel)          // Sum
		force := ph.CalcForces()
		switch {
		case angle > 32767:
			output -= SoftLockForce * (angle - 32767)
		case angle < -32767:
			output -= SoftLockForce * (angle + 32767)
		}
		output -= force[0]
		cnt++
		/*
			if cnt%100 == 0 {
				log.Printf("angle:%d, verocity:%d, cogl:%d, decel:%d, o:%d", angle, verocity, cog, decel, output)
			}
		*/
		// for slow start
		if cnt < 300 {
			output = output * int32(cnt) / 300
		}
		if err := motor.Output(can, int16(limit1(output))); err != nil {
			log.Print(err)
		}
		if !dummyMode {
			js.SetButton(4, false)
			js.SetButton(5, false)
			js.SetButton(6, angle < -32767)
			js.SetButton(7, angle > 32767)
			js.SetAxis(0, int(limit1(angle)))
			js.SetAxis(5, int(limit1(angle)))
		}
		if cnt%10 == 0 {
			js.SendState()
		}
	}
}
