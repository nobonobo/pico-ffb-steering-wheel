package main

import (
	"bufio"
	"context"
	"log"
	"machine"
	"os"
	"strconv"
	"strings"
	"time"

	"tinygo.org/x/drivers/mcp2515"

	"diy-ffb-wheel/control"
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
)

var (
	spi = machine.SPI0
)

func init() {
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

func getShift(x, y int32) int {
	dx, dy := limitx(fitx(x)), limity(fity(y))
	s := shift[dx][dy]
	return s
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
	can := mcp2515.New(spi, CAN_CS)
	can.Configure()
	if err := can.Begin(mcp2515.CAN500kBps, mcp2515.Clock8MHz); err != nil {
		log.Fatal(err)
	}
	js := control.NewWheel(can)
	go func() {
		time.Sleep(1 * time.Second)
		axises := make([]int32, 11)
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			for i, s := range strings.Split(scanner.Text(), ",") {
				if i >= len(axises) {
					break
				}
				v, err := strconv.Atoi(s)
				if err != nil {
					break
				}
				axises[i] = int32(v)
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
			}
			shift := getShift(axises[0], axises[1])
			// for sequential mode
			switch {
			case axises[7] > 0:
				js.SetButton(8, true)
			case axises[6] > 0:
				js.SetButton(9, true)
			default:
				js.SetButton(8, false)
				js.SetButton(9, false)
				const begin = 10
				for i := 1; i < 9; i++ {
					if i == shift {
						js.SetButton(i+begin-1, true)
					} else {
						js.SetButton(i+begin-1, false)
					}
				}
			}
			if shift == 0 {
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
		if err := scanner.Err(); err != nil {
			log.Print(err)
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for {
		if err := js.Loop(ctx); err != nil {
			log.Print(err)
			time.Sleep(3 * time.Second)
		}
	}
}
