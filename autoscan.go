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

	"code.google.com/p/goauth2/oauth"
	drive "code.google.com/p/google-api-go-client/drive/v2"

	"github.com/ThomasHabets/autoscan/backend"
	"github.com/ThomasHabets/autoscan/backend/leds"
	"github.com/ThomasHabets/autoscan/buttons"
	"github.com/ThomasHabets/autoscan/web"
)

var (
	listen     = flag.String("listen", "", "Address to listen to.")
	socketPath = flag.String("socket", "", "UNIX socket to listen to.")

	logfile    = flag.String("logfile", "", "Where to log. If not specified will log to stdout.")
	configFile = flag.String("config", ".autoscan", "Config file.")
	tmplDir    = flag.String("templates", "", "Directory with HTML templates.")
	staticDir  = flag.String("static", "", "Directory with static files.")

	// Externals
	scanimage = flag.String("scanimage", "scanimage", "Scanimage binary from SANE.")
	convert   = flag.String("convert", "convert", "Convert binary from ImageMagick.")

	pinButtonSingle = flag.Int("pin_single", 22, "GPIO PIN for 'scan single'.")
	pinButtonDuplex = flag.Int("pin_duplex", 23, "GPIO PIN for 'scan duplex'.")
	pinButton3      = flag.Int("pin_ack", 17, "GPIO PIN for 'ACK'.")
	pinButton4      = flag.Int("pin_reboot", 27, "GPIO PIN for 'reboot'.")

	pinLED1a = flag.Int("pin_led1_a", 6, "GPIO PIN for LED 1 PIN 1/2.")
	pinLED1b = flag.Int("pin_led1_b", 25, "GPIO PIN for LED 1 PIN 2/2.")

	pinLED2a = flag.Int("pin_led2_a", 5, "GPIO PIN for LED 2 PIN 1/2.")
	pinLED2b = flag.Int("pin_led2_b", 24, "GPIO PIN for LED 2 PIN 2/2.")
)

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

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	if *logfile != "" {
		f, err := os.OpenFile(*logfile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0660)
		if err != nil {
			log.Fatalf("Opening logfile %q: %v", *logfile, err)
		}
		log.SetOutput(f)
	}

	if *staticDir == "" {
		log.Fatalf("-static is mandatory.")
	}
	if *tmplDir == "" {
		log.Fatalf("-templates is mandatory.")
	}

	if (*listen == "") == (*socketPath == "") {
		log.Fatalf("Exactly one of -listen and -socket must be specified.")
	}

	// Status LED: Blink when this daemon is running.
	status := make(chan leds.LEDMode)
	_, err := leds.LEDController(*pinLED1a, *pinLED1b, status)
	if err != nil {
		log.Fatalf("Status LED: %v", err)
	}
	status <- leds.GREEN
	status <- leds.BLINK

	// Progress LED:
	// * Solid green or red showing last status, ready for new scan.
	// * Blinking green while "in progress".
	progress := make(chan leds.LEDMode)
	_, err = leds.LEDController(*pinLED2a, *pinLED2b, progress)
	if err != nil {
		log.Fatalf("Progress LED: %v", err)
	}
	progress <- leds.GREEN

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

	b := backend.Backend{
		Scanimage: *scanimage,
		Convert:   *convert,
		ParentDir: cfg.parent,
		Drive:     d,
		Progress:  progress,
	}
	b.Init()

	f := web.New(*tmplDir, *staticDir)
	f.Backend = &b

	btns, err := buttons.New(*pinButtonSingle, *pinButtonDuplex, *pinButton3, *pinButton4)
	if err != nil {
		log.Fatalf("Setting up buttons: %v", err)
	}
	btns.Backend = &b
	btns.Progress = progress

	log.Printf("Running.")
	go btns.Run()

	servePort(f.Mux)
}
