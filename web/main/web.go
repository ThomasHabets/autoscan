package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
	"strings"
	"time"

	"code.google.com/p/goauth2/oauth"
	drive "code.google.com/p/google-api-go-client/drive/v2"
	"github.com/davecheney/gpio"

	"github.com/ThomasHabets/autoscan/web"
)

var (
	socketPath = flag.String("socket", "", "UNIX socket to listen to.")
	listen     = flag.String("listen", ":8080", "Address to listen to.")
	configFile = flag.String("config", ".autoscan", "Config file.")
	tmplDir    = flag.String("templates", "", "Directory with HTML templates.")
	staticDir  = flag.String("static", "", "Directory with static files.")

	pinButtonSingle = flag.Int("pin_single", 22, "GPIO PIN for 'scan single'.")
	pinButtonDuplex = flag.Int("pin_duplex", 23, "GPIO PIN for 'scan duplex'.")

	pinLED1a = flag.Int("pin_led1_a", 6, "GPIO PIN for LED 1 PIN 1/2.")
	pinLED1b = flag.Int("pin_led1_b", 25, "GPIO PIN for LED 1 PIN 2/2.")

	pinLED2a = flag.Int("pin_led2_a", 5, "GPIO PIN for LED 2 PIN 1/2.")
	pinLED2b = flag.Int("pin_led2_b", 24, "GPIO PIN for LED 2 PIN 2/2.")
)

type LEDMode string

const (
	RED   LEDMode = "RED"
	GREEN LEDMode = "GREEN"
	BLINK LEDMode = "BLINK"
	OFF   LEDMode = "OFF"
)

func LEDController(a, b int, control <-chan LEDMode) {
	mode := GREEN
	blink := false
	blinkOn := false
	ledA, err := gpio.OpenPin(a, gpio.ModeOutput)
	if err != nil {
		log.Fatalf("Opening LED pin %d: %v", a, err)
	}
	ledB, err := gpio.OpenPin(b, gpio.ModeOutput)
	if err != nil {
		log.Fatalf("Opening heartbeat LED pin %d: %v", b, err)
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
	for {
		select {
		case <-time.After(time.Second):
		case m := <-control:
			if m == BLINK {
				blink = true
			} else {
				blink = false
				mode = m
			}
		}
		maybe()
	}
}

func oauthConfig(id, secret string) *oauth.Config {
	return &oauth.Config{
		ClientId:     id,
		ClientSecret: secret,
		AuthURL:      "https://accounts.google.com/o/oauth2/auth",
		Scope:        "https://www.googleapis.com/auth/drive",
		//Scope:       "https://www.googleapis.com/auth/drive.file",
		TokenURL:    "https://accounts.google.com/o/oauth2/token",
		RedirectURL: "urn:ietf:wg:oauth:2.0:oob",
		AccessType:  "offline",
	}
}

func serveFCGI(m *http.ServeMux) error {
	sock, err := net.Listen("unix", *socketPath)
	if err != nil {
		log.Fatal("Unable to listen to socket: ", err)
	}
	if err = os.Chmod(*socketPath, 0666); err != nil {
		log.Fatal("Unable to chmod socket: ", err)
	}

	return fcgi.Serve(sock, m)
}

func servePort(m *http.ServeMux) error {
	return http.ListenAndServe(*listen, m)
}

type config struct {
	clientID, clientSecret, refreshToken string
	parent                               string
}

func readConfig() (*config, error) {
	b, err := ioutil.ReadFile(*configFile)
	if err != nil {
		return nil, err
	}
	s := strings.SplitN(strings.Trim(string(b), "\n\r "), "\n", 4)
	return &config{
		clientID:     s[0],
		clientSecret: s[1],
		refreshToken: s[2],
		parent:       s[3],
	}, nil
}
func connect(id, secret, token string) (*oauth.Transport, error) {
	t := &oauth.Transport{
		Config: oauthConfig(id, secret),
		Token: &oauth.Token{
			RefreshToken: token,
		},
	}
	return t, t.Refresh()
}
func main() {
	flag.Parse()

	status := make(chan LEDMode)
	go LEDController(*pinLED1a, *pinLED1b, status)
	progress := make(chan LEDMode)
	go LEDController(*pinLED2a, *pinLED2b, progress)
	status <- GREEN
	status <- BLINK
	progress <- RED
	progress <- BLINK

	cfg, err := readConfig()
	if err != nil {
		log.Fatal(err)
	}
	t, err := connect(cfg.clientID, cfg.clientSecret, cfg.refreshToken)
	if err != nil {
		log.Fatal(err)
	}
	d, err := drive.New(t.Client())
	if err != nil {
		log.Fatalf("Creating Google Drive client: %v", err)
	}
	f := web.New(*tmplDir, *staticDir, d, cfg.parent)
	log.Printf("Running.")
	servePort(f.Mux)
}
