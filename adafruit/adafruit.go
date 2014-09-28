package adafruit

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/ThomasHabets/autoscan/backend"
)

type adafruit struct {
	b      *backend.Backend
	cmd    *exec.Cmd
	stdin  io.Writer
	stdout io.Reader
}

// New creates a new adafruit object.
func New(b *backend.Backend) (*adafruit, error) {
	cmd := exec.Command("/opt/autoscan/bin/lcd.py")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to create stdout pipe: %v", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Fatalf("Failed to create stdin pipe: %v", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start lcd binary: %v", err)
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
			a.Msg("IDLE", "Autoscan ready|")
		}
	}
}
