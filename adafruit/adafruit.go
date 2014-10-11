// Package adafruit implements a UI against the Adafruit 16x2 LCD display.
//
// https://learn.adafruit.com/adafruit-16x2-character-lcd-plus-keypad-for-raspberry-pi/overview
//
// 'Select' button scans single-sided.
// 'Right' button scans double-sided.
// 'Up' button resets (acks) error message.
package adafruit

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/ThomasHabets/autoscan/backend"
)

var (
	adafruitLCDBinary = flag.String("adafruit_lcd_binary", "/opt/autoscan/bin/lcd.py", "Path to LCD.py binary.")
)

// adafruit implements the backend.UI interface.
type adafruit struct {
	b      *backend.Backend
	cmd    *exec.Cmd
	stdin  io.Writer
	stdout io.Reader
}

// New creates a new adafruit object.
func New(b *backend.Backend) (*adafruit, error) {
	cmd := exec.Command(*adafruitLCDBinary)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdout pipe: %v", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdin pipe: %v", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting lcd binary %q: %v", *adafruitLCDBinary, err)
	}
	return &adafruit{
		b:      b,
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
	}, nil
}

func (a *adafruit) Msg(status, msg string) {
	colors := "1|0|0"
	switch status {
	case "FAILED":
		colors = "1|0|0"
	case "IDLE":
		colors = "0|1|0"
	case "ACTIVE":
		colors = "0|0|1"
	}
	fmt.Fprintf(a.stdin, "%s|%s\n", colors, msg)
}

func (a *adafruit) Run() {
	for {
		reader := bufio.NewReader(a.stdout)
		l, err := reader.ReadString('\n')
		if err != nil {
			log.Fatalf("Reading from LCD process: %v", err)
		}
		l = strings.Trim(l, "\n ")
		switch l {
		case "SELECT":
			a.b.Run(false)
		case "RIGHT":
			a.b.Run(true)
		case "UP":
			// Clear error message.
			a.Msg("IDLE", "Autoscan ready|")
		}
	}
}
