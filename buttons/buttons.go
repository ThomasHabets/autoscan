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
)

type Buttons struct {
	Backend *backend.Backend

	Duplex *Input
	Single *Input
	ACK    *Input
	Reboot *Input
}

type button int

const (
	// Buttons
	SINGLE button = iota
	DUPLEX
	ACK
	REBOOT
)

const (
	BasePath = "/sys/class/gpio"
)

type Input struct {
	File *os.File
}

func OpenInput(n int) (*Input, error) {
	f, err := os.Open(path.Join(BasePath, fmt.Sprintf("gpio%d", n), "value"))
	if err != nil {
		return nil, err
	}
	return &Input{
		File: f,
	}, nil
}

func New(s, b, a, r int) (*Buttons, error) {
	ret := &Buttons{}
	var err error

	ret.Single, err = OpenInput(s)
	if err != nil {
		return nil, fmt.Errorf("opening single pin %d: %v", a, err)
	}

	ret.Duplex, err = OpenInput(b)
	if err != nil {
		return nil, fmt.Errorf("opening duplex pin %d: %v", a, err)
	}

	ret.ACK, err = OpenInput(a)
	if err != nil {
		return nil, fmt.Errorf("opening ACK pin %d: %v", a, err)
	}

	ret.Reboot, err = OpenInput(r)
	if err != nil {
		return nil, fmt.Errorf("opening reboot pin %d: %v", a, err)
	}

	return ret, nil
}

// wait for a button to be pressed, and return that button.
func (b *Buttons) waitButton() button {
	btns := map[button]*Input{
		SINGLE: b.Single,
		DUPLEX: b.Duplex,
		ACK:    b.ACK,
		REBOOT: b.Reboot,
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

func (b *Buttons) Run() {
	log.Printf("Starting main loop.")
	for {
		log.Printf("Main loop iteration.")
		btn := b.waitButton()
		switch btn {
		case SINGLE:
			log.Printf("SINGLE button pressed.")
			b.Backend.Run(false)
		case DUPLEX:
			log.Printf("DUPLEX button pressed.")
			b.Backend.Run(true)
		case ACK:
			log.Printf("ACK button pressed needlessly.")
		case REBOOT:
			log.Printf("REBOOT button pressed.")
		}
		time.Sleep(time.Second)
	}
}
