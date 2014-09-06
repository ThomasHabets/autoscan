package leds

import (
	"fmt"
	"time"

	"github.com/davecheney/gpio"
)

type LEDMode string

const (
	RED      LEDMode = "RED"
	GREEN    LEDMode = "GREEN"
	BLINK    LEDMode = "BLINK"
	OFF      LEDMode = "OFF"
	SHUTDOWN LEDMode = "SHUTDOWN"
)

func LEDController(a, b int, control <-chan LEDMode) (<-chan struct{}, error) {
	mode := GREEN
	blink := false
	blinkOn := false
	ledA, err := gpio.OpenPin(a, gpio.ModeOutput)
	if err != nil {
		return nil, fmt.Errorf("opening LED pin %d: %v", a, err)
	}
	ledB, err := gpio.OpenPin(b, gpio.ModeOutput)
	if err != nil {
		return nil, fmt.Errorf("opening heartbeat LED pin %d: %v", b, err)
	}
	maybe := func() {
		blinkOn = !blinkOn
		if blink && !blinkOn {
			ledA.Clear()
			ledB.Clear()
			return
		}
		switch mode {
		case RED:
			ledA.Set()
			ledB.Clear()
		case GREEN:
			ledA.Clear()
			ledB.Set()
		case OFF:
			ledA.Clear()
			ledB.Clear()
		}
	}
	done := make(chan struct{})
	go func() {
		defer func() {
			ledA.Close()
			ledB.Close()
			close(done)
		}()

		for {
			select {
			case <-time.After(time.Second / 2):
			case m := <-control:
				switch m {
				case SHUTDOWN:
					return
				case BLINK:
					blink = true
				default:
					blink = false
					mode = m
				}
			}
			maybe()
		}
	}()
	return done, nil
}
