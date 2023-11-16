package main

import (
	"bufio"
	"context"
	"machine"
	"os"
	"strconv"
	"strings"
	"time"

	"tinygo.org/x/drivers/mcp2515"

	"github.com/SWITCHSCIENCE/ffb_steering_controller/control"
	"github.com/SWITCHSCIENCE/ffb_steering_controller/settings"
	"github.com/SWITCHSCIENCE/ffb_steering_controller/utils"
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
	}
	shift = [][]int{
		0: {2, 0, 1},
		1: {4, 0, 3},
		2: {6, 0, 5},
		3: {8, 0, 7},
	}
	limitx         = utils.Limit(-1, 2)
	limity         = utils.Limit(-1, 1)
	sequentialMode = true
)

func getShift(x, y int32) int {
	dx, dy := limitx(x)+1, limity(y)+1
	s := shift[dx][dy]
	return s
}

var (
	sw [3]bool
)

func update() {
	s := settings.Get()
	now := [3]bool{
		!SW1.Get(),
		!SW2.Get(),
		!SW3.Get(),
	}
	active := [3]bool{
		now[0] && !sw[0],
		now[1] && !sw[1],
		now[2] && !sw[2],
	}
	copy(sw[:], now[:])
	current := s.Lock2Lock
	next := current
	switch {
	case active[2]:
		switch current {
		case 1080:
		case 720:
			next = 1080
		case 540:
			next = 720
		case 360:
			next = 540
		case 180:
			next = 360
		}
	case active[0]:
		switch s.Lock2Lock {
		case 1080:
			next = 720
		case 720:
			next = 540
		case 540:
			next = 360
		case 360:
			next = 180
		case 180:
		}
	}
	switch next {
	case 1080:
		LED1.Low()
		LED2.High()
		LED3.High()
	case 720:
		LED1.Low()
		LED2.Low()
		LED3.High()
	case 540:
		LED1.High()
		LED2.Low()
		LED3.High()
	case 360:
		LED1.High()
		LED2.Low()
		LED3.Low()
	case 180:
		LED1.High()
		LED2.High()
		LED3.Low()
	}
	if s.Lock2Lock != next {
		s.Lock2Lock = next
		settings.Update(s)
	}
}

func main() {
	LED1.Low()
	if err := spi.Configure(
		machine.SPIConfig{
			Frequency: 500000,
			SCK:       CAN_SCK,
			SDO:       CAN_TX,
			SDI:       CAN_RX,
			Mode:      0,
		},
	); err != nil {
		panic(err)
	}
	can := mcp2515.New(spi, CAN_CS)
	can.Configure()
	if err := can.Begin(mcp2515.CAN500kBps, mcp2515.Clock8MHz); err != nil {
		panic(err)
	}
	js := control.NewWheel(can)
	go func() {
		defer func() {
			if err := recover(); err != nil {
				println(err)
			}
		}()
		for {
			time.Sleep(1 * time.Second)
			axises := make([]int32, 11)
			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				args := strings.Split(scanner.Text(), ",")
				if len(args) != 6 {
					println("length mismatch:", len(args))
					continue
				}
				for i, s := range args {
					if i >= len(axises) {
						println("length mismatch:", i)
						continue
					}
					v, err := strconv.Atoi(s)
					if err != nil {
						println(err)
						continue
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
				if axises[0] != 0 {
					sequentialMode = false
				}
				// for sequential mode
				if sequentialMode {
					switch {
					case axises[1] > 0:
						js.SetButton(8, true)
					case axises[1] < 0:
						js.SetButton(9, true)
					default:
						js.SetButton(8, false)
						js.SetButton(9, false)
					}
				} else {
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
				println(err)
			}
		}
	}()
	s := settings.Get()
	s.MaxCenteringForce = 50
	settings.Update(s)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		tick := time.NewTicker(20 * time.Millisecond)
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				update()
			}
		}
	}()
	defer cancel()
	for {
		if err := js.Loop(ctx); err != nil {
			println(err)
			time.Sleep(3 * time.Second)
		}
	}
}
