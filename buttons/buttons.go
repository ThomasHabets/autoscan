package buttons

/*

Buttons
 Duplex    Starts autoscan -feeder.
 Single   Starts autoscan in single-page mode. Autoscan handles the UI from there.

Undecided:
 ACK      If something goes wrong the status LED will blink until ACK is pressed.
 Reboot   Reboots the raspberry pi.

*/
import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path"
	"time"

	"github.com/ThomasHabets/autoscan/backend"
	"github.com/ThomasHabets/autoscan/backend/leds"
)

// Buttons keeps track of the buttons and notifies backend and LEDs.
type Buttons struct {
	Backend  *backend.Backend
	Progress chan<- leds.LEDMode

	Duplex *input
	Single *input
	ACK    *input
	Reboot *input
}

type button int

const (
	single button = iota // Scans single-sided pages.
	duplex               // Scans double-sided pages.
	ack                  // Turns a red lamp green.
	reboot               // Reboots the machine.
)

const (
	// BasePath is where are the GPIO special files are.
	BasePath = "/sys/class/gpio"
)

type input struct {
	File *os.File
}

func openInput(n int) (*input, error) {
	// Export.
	if err := func() error {
		f, err := os.OpenFile(path.Join(BasePath, "export"), os.O_WRONLY, 0660)
		if err != nil {
			return err
		}
		defer f.Close()
		fmt.Fprintf(f, "%d\n", n)
		return nil
	}(); err != nil {
		return nil, err
	}

	// Direction.
	if err := func() error {
		f, err := os.OpenFile(path.Join(BasePath, fmt.Sprintf("gpio%d", n), "direction"), os.O_WRONLY, 0660)
		if err != nil {
			return err
		}
		defer f.Close()
		fmt.Fprintf(f, "in\n")
		return nil
	}(); err != nil {
		return nil, err
	}

	f, err := os.Open(path.Join(BasePath, fmt.Sprintf("gpio%d", n), "value"))
	if err != nil {
		return nil, err
	}
	return &input{
		File: f,
	}, nil
}

// New opens GPIO pins and creates a new Buttons.
func New(s, b, a, r int) (*Buttons, error) {
	ret := &Buttons{}
	var err error

	ret.Single, err = openInput(s)
	if err != nil {
		return nil, fmt.Errorf("opening single pin %d: %v", a, err)
	}

	ret.Duplex, err = openInput(b)
	if err != nil {
		return nil, fmt.Errorf("opening duplex pin %d: %v", a, err)
	}

	ret.ACK, err = openInput(a)
	if err != nil {
		return nil, fmt.Errorf("opening ACK pin %d: %v", a, err)
	}

	ret.Reboot, err = openInput(r)
	if err != nil {
		return nil, fmt.Errorf("opening reboot pin %d: %v", a, err)
	}

	return ret, nil
}

// wait for a button to be pressed, and return that button.
func (b *Buttons) waitButton() button {
	btns := map[button]*input{
		single: b.Single,
		duplex: b.Duplex,
		ack:    b.ACK,
		reboot: b.Reboot,
	}
	// Poll for button status.
	// This is an ugly ugly hack, but the 'gpio' package is unstable.
	// TODO: Switch to edge triggering, or a stable package that does edge triggering.
	// This loop takes around 1% CPU. Crazy, I know.
	for {
		time.Sleep(100 * time.Millisecond)
		for k, v := range btns {
			v.File.Seek(0, 0)
			r := bufio.NewReader(v.File)
			l, _ := r.ReadString('\n')
			if len(l) > 0 && l[0] == '1' {
				return k
			}
		}
	}
}

// Run listens to button presses. Forever.
func (b *Buttons) Run() {
	log.Printf("Starting button reading loop.")
	for {
		btn := b.waitButton()
		switch btn {
		case single:
			log.Printf("SINGLE button pressed.")
			b.Backend.Run(false)
		case duplex:
			log.Printf("DUPLEX button pressed.")
			b.Backend.Run(true)
		case ack:
			log.Printf("ACK button pressed.")
			b.Progress <- leds.GREEN
		case reboot:
			log.Printf("REBOOT button pressed.")
		}
		time.Sleep(time.Second)
	}
}
