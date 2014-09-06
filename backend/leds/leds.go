package leds

import (
	"fmt"
	"time"

	"github.com/davecheney/gpio"
)

// LEDMode is a what the LED is doing.
// TODO: This is a bit ugly and untrue since blinking is orthogonal to colour.
type LEDMode string

// LEDMode "commands".
const (
	RED      LEDMode = "RED"
	GREEN    LEDMode = "GREEN"
	BLINK    LEDMode = "BLINK"
	OFF      LEDMode = "OFF"
	SHUTDOWN LEDMode = "SHUTDOWN"
)

// LEDController opens GPIO ports and runs forever, keeping this LED up to date.
// Send SHUTDOWN to stop this goroutine.
// Returns a channel notifying that it's done and won't be touching the GPIO ports anymore.
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
