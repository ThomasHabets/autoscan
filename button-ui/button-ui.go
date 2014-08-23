package main

/*

LEDs
 Status
   Solid      At root menu.
   Blinking   Something went wrong. Press ACK button to confirm and reset.
   Off        Not awaiting any instructions. E.g. "currently scanning".
 Heartbeat
   Blinking   This program is active. (even when shelling out to autoscan)

Buttons
 Batch    Starts autoscan -feeder.
 Single   Starts autoscan in single-page mode. Autoscan handles the UI from there.
 ACK      If something goes wrong the status LED will blink until ACK is pressed.
 Reboot   Reboots the raspberry pi.


*/
// (add-hook 'before-save-hook 'gofmt-before-save)
import (
	"flag"
	"log"
	"os/exec"
	"time"

	"github.com/davecheney/gpio"
)

var (
	// Port config.
	ledPort       = flag.Int("led_port", 17, "Information status LED port.")
	heartbeatPort = flag.Int("heartbeat_port", 25, "Heartbeat LED port.")
	batchPort     = flag.Int("batch_port", 21, "Batch button port.")
	singlePort    = flag.Int("single_port", 22, "Single-scan button port.")
	ackPort       = flag.Int("ack_port", 23, "Ack port.")
	rebootPort    = flag.Int("reboot_port", 24, "Reboot button port.")

	autoscan = flag.String("autoscan", "autoscan", "Autoscan binary.")
	name     = flag.String("name", "raspberry-scan", "Name of folder on Google Drive.")

	led          gpio.Pin
	batchButton  gpio.Pin
	singleButton gpio.Pin
	ackButton    gpio.Pin
	rebootButton gpio.Pin
)

type button int

const (
	// Buttons
	BATCH button = iota
	SINGLE
	ACK
	REBOOT

	trigger = gpio.EdgeFalling
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
	batchButton.BeginWatch(trigger, func() { w <- BATCH })
	defer batchButton.EndWatch()
	singleButton.BeginWatch(trigger, func() { w <- SINGLE })
	defer singleButton.EndWatch()
	ackButton.BeginWatch(trigger, func() { w <- ACK })
	defer ackButton.EndWatch()
	rebootButton.BeginWatch(trigger, func() { w <- REBOOT })
	defer rebootButton.EndWatch()
	log.Printf("Waiting for button")
	return <-w
}

func waitAck() {
	log.Printf("Waiting for ACK.")
	log.Printf("Got ACK.")
	w := make(chan bool)
	ackButton.BeginWatch(trigger, func() { w <- true })
	defer ackButton.EndWatch()
	<-w
}

func runAutoscan(moarArgs ...string) error {
	args := append([]string{
		"-name", *name,
	}, moarArgs...)
	cmd := exec.Command(*autoscan, args...)
	return cmd.Run()
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
			if err := runAutoscan("-feeder"); err != nil {
				log.Printf("BATCH autoscan failed: %v", err)
				waitAck()
			}
		case SINGLE:
			log.Printf("SINGLE button pressed.")
			if err := runAutoscan("-next_button", *singlePort); err != nil {
				log.Printf("SINGLE autoscan failed: %v", err)
				waitAck()
			}
		case ACK:
			log.Printf("ACK button pressed needlessly.")
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
	hb, err := gpio.OpenPin(*heartbeatPort, gpio.ModeOutput)
	if err != nil {
		log.Fatalf("Opening heartbeat LED pin %d: %v", *heartbeatPort, err)
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
	ackButton, err = gpio.OpenPin(*ackPort, gpio.ModeInput)
	if err != nil {
		log.Fatalf("Opening ack button pin %d: %v", *ackPort, err)
	}
	go heartbeats(hb)
	mainLoop()
}
