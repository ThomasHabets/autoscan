package main

// (add-hook 'before-save-hook 'gofmt-before-save)
import (
	"flag"
	"log"
	"time"

	"github.com/davecheney/gpio"
)

var (
	ledPort       = flag.Int("led_port", 25, "Information status LED port.")
	heartbeatPort = flag.Int("heartbeat_port", 17, "Heartbeat LED port.")
	batchPort     = flag.Int("batch_port", 27, "Batch button port.")
	singlePort    = flag.Int("single_port", 22, "Single-scan button port.")
	unusedPort    = flag.Int("unused_port", 23, "Unused port.")
	rebootPort    = flag.Int("reboot_port", 24, "Reboot button port.")

	led          gpio.Pin
	batchButton  gpio.Pin
	singleButton gpio.Pin
	unusedButton gpio.Pin
	rebootButton gpio.Pin
)

type button int

const (
	// Buttons
	BATCH button = iota
	SINGLE
	UNUSED
	REBOOT
)

func heartbeats(hb gpio.Pin) {
	for {
		hb.Set()
		time.Sleep(time.Second)
		hb.Clear()
		time.Sleep(time.Second)
	}
}

func waitButton() button {
	w := make(chan button, 100)
	batchButton.BeginWatch(gpio.EdgeFalling, func() { w <- BATCH })
	defer batchButton.EndWatch()
	singleButton.BeginWatch(gpio.EdgeFalling, func() { w <- SINGLE })
	defer singleButton.EndWatch()
	unusedButton.BeginWatch(gpio.EdgeFalling, func() { w <- UNUSED })
	defer unusedButton.EndWatch()
	rebootButton.BeginWatch(gpio.EdgeFalling, func() { w <- REBOOT })
	defer rebootButton.EndWatch()
	return <-w
}

func mainLoop() {
	log.Printf("Starting main loop.")
	for {
		log.Printf("Main loop iteration.")
		led.Set()
		btn := waitButton()
		led.Clear()
		switch btn {
		case BATCH:
			log.Printf("BATCH button pressed.")
		case SINGLE:
			log.Printf("SINGLE button pressed.")
		case UNUSED:
			log.Printf("UNUSED button pressed.")
		case REBOOT:
			log.Printf("REBOOT button pressed.")
		}
		time.Sleep(time.Second)
	}
}

func main() {
	flag.Parse()

	var err error
	led, err = gpio.OpenPin(*ledPort, gpio.ModeOutput)
	if err != nil {
		log.Fatalf("Opening LED pin %d: %v", *ledPort, err)
	}

	batchButton, err = gpio.OpenPin(*batchPort, gpio.ModeInput)
	if err != nil {
		log.Fatalf("Opening batch button pin %d: %v", *batchPort, err)
	}
	singleButton, err = gpio.OpenPin(*singlePort, gpio.ModeInput)
	if err != nil {
		log.Fatalf("Opening single button pin %d: %v", *singlePort, err)
	}
	rebootButton, err = gpio.OpenPin(*rebootPort, gpio.ModeInput)
	if err != nil {
		log.Fatalf("Opening reboot button pin %d: %v", *rebootPort, err)
	}
	hb, err := gpio.OpenPin(*heartbeatPort, gpio.ModeOutput)
	if err != nil {
		log.Fatalf("Opening heartbeat LED pin %d: %v", *heartbeatPort, err)
	}
	go heartbeats(hb)
	mainLoop()
}
