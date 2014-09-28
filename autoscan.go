/*
Autoscan - Automate scanning from USB to Google Drive using a Raspberry Pi.

Example usage:
    ./autoscan \
        -templates src/github.com/ThomasHabets/autoscan/web/templates/ \
        -scanimage $(pwd)/src/github.com/ThomasHabets/autoscan/extra/scanimage-wrap \
        -static src/github.com/ThomasHabets/autoscan/web/static/ \
        -listen :8080
*/
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
	"path"
	"strings"
	"time"

	"code.google.com/p/goauth2/oauth"
	drive "code.google.com/p/google-api-go-client/drive/v2"
	drivedulib "github.com/ThomasHabets/drive-du/lib"

	"github.com/ThomasHabets/autoscan/adafruit"
	"github.com/ThomasHabets/autoscan/backend"
	//"github.com/ThomasHabets/autoscan/backend/leds"
	"github.com/ThomasHabets/autoscan/buttons"
	"github.com/ThomasHabets/autoscan/web"
)

const (
	// BasePath is where are the GPIO special files are.
	BasePath = "/sys/class/gpio"
	scope    = "https://www.googleapis.com/auth/drive"
)

var (
	listen     = flag.String("listen", "", "Address to listen to.")
	listenFCGI = flag.String("listen_fcgi", "", "FCGI Address to listen to.")
	socketPath = flag.String("socket", "", "UNIX socket to listen to for FCGI.")

	logfile    = flag.String("logfile", "", "Where to log. If not specified will log to stdout.")
	configFile = flag.String("config", ".autoscan", "Config file.")
	configure  = flag.Bool("configure", false, "Create config file.")
	tmplDir    = flag.String("templates", "", "Directory with HTML templates.")
	staticDir  = flag.String("static", "", "Directory with static files.")

	useButtons  = flag.Bool("use_buttons", false, "Enable buttons.")
	useLEDs     = flag.Bool("use_leds", false, "Use LEDs.")
	useAdafruit = flag.Bool("use_adafruit", false, "Use Adafruit 16x2 LCD display.")

	// Externals
	scanimage = flag.String("scanimage", "scanimage", "Scanimage binary from SANE.")
	convert   = flag.String("convert", "convert", "Convert binary from ImageMagick.")

	pinButtonSingle = flag.Int("pin_single", 5, "GPIO PIN for 'scan single'.")
	pinButtonDuplex = flag.Int("pin_duplex", 6, "GPIO PIN for 'scan duplex'.")
	pinButton3      = flag.Int("pin_ack", 24, "GPIO PIN for 'ACK'.")
	pinButton4      = flag.Int("pin_reboot", 25, "GPIO PIN for 'reboot'.")

	pinLED1a = flag.Int("pin_led1_a", 27, "GPIO PIN for LED 1 PIN 1/2.")
	pinLED1b = flag.Int("pin_led1_b", 23, "GPIO PIN for LED 1 PIN 2/2.")

	pinLED2a = flag.Int("pin_led2_a", 17, "GPIO PIN for LED 2 PIN 1/2.")
	pinLED2b = flag.Int("pin_led2_b", 22, "GPIO PIN for LED 2 PIN 2/2.")
)

func oauthConfig(id, secret string) *oauth.Config {
	return &oauth.Config{
		ClientId:     id,
		ClientSecret: secret,
		AuthURL:      "https://accounts.google.com/o/oauth2/auth",
		Scope:        scope,
		TokenURL:     "https://accounts.google.com/o/oauth2/token",
		RedirectURL:  "urn:ietf:wg:oauth:2.0:oob",
		AccessType:   "offline",
	}
}

func serveFCGI(m *http.ServeMux) error {
	if err := os.Remove(*socketPath); err != nil {
		log.Printf("Removing old socket %q: %v", *socketPath, err)
	}
	sock, err := net.Listen("unix", *socketPath)
	if err != nil {
		log.Fatal("Unable to listen to socket: ", err)
	}
	if err = os.Chmod(*socketPath, 0666); err != nil {
		log.Fatal("Unable to chmod socket: ", err)
	}

	return fcgi.Serve(sock, m)
}

func serveFCGIPort(m *http.ServeMux) error {
	sock, err := net.Listen("tcp", *listenFCGI)
	if err != nil {
		log.Fatalf("Unable to listen to socket %q: %v", *listenFCGI, err)
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

func export(n int) error {
	if err := func() error {
		f, err := os.OpenFile(path.Join(BasePath, "export"), os.O_WRONLY, 0660)
		if err != nil {
			return err
		}
		defer f.Close()
		fmt.Fprintf(f, "%d\n", n)
		return nil
	}(); err != nil {
		return err
	}
	start := time.Now()
	for {
		time.Sleep(50 * time.Millisecond)
		_, err := os.Stat(path.Join(BasePath, fmt.Sprintf("gpio%d", n), "direction"))
		if err != nil {
			if time.Since(start) > 10*time.Second {
				return err
			}
			continue
		}
		_, err = os.Stat(path.Join(BasePath, fmt.Sprintf("gpio%d", n), "value"))
		if err != nil {
			if time.Since(start) > 10*time.Second {
				return err
			}
			continue
		}
		return nil
	}
}

func setDirection(n int, dir string) error {
	start := time.Now()
	if err := func() error {
		for {
			f, err := os.OpenFile(path.Join(BasePath, fmt.Sprintf("gpio%d", n), "direction"), os.O_WRONLY, 0660)
			if err != nil {
				if time.Since(start) > 10*time.Second {
					return err
				}
				time.Sleep(50 * time.Millisecond)
				continue
			}
			defer f.Close()
			fmt.Fprintf(f, dir+"\n")
			return nil
		}
	}(); err != nil {
		return err
	}
	for {
		s, err := ioutil.ReadFile(path.Join(BasePath, fmt.Sprintf("gpio%d", n), "direction"))
		if err != nil {
			return err
		}
		if string(s) == dir+"\n" {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func main() {
	flag.Parse()

	if *configFile == "" {
		log.Fatalf("-config is mandatory")
	}
	if *configure {
		conf, err := drivedulib.Configure(scope, "offline")
		if err != nil {
			log.Fatalf("Failed to configure: %v", err)
		}
		folder, err := drivedulib.ReadLine("Folder ID: ")
		if err != nil {
			log.Fatalf("Unable to read folder ID: %v", err)
		}
		if err := ioutil.WriteFile(*configFile, []byte(fmt.Sprintf("%s\n%s\n%s\n%s\n", conf.ID, conf.Secret, conf.Token, folder)), 0600); err != nil {
			log.Fatalf("Failed to write config file: %v", err)
		}
		return
	}

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

	{
		a, b, c := *listen != "", *listenFCGI != "", *socketPath != ""
		if (a && (b || c)) || (b && (a || c)) || (c && (a || b)) {
			log.Fatalf("Exactly one of -listen, listen_fcgi and -socket must be specified.")
		}
	}

	// Set up GPIO ports race-free.
	for _, n := range []int{
		*pinLED1a,
		*pinLED1b,
		*pinLED2a,
		*pinLED2b,
	} {
		if err := export(n); err != nil {
			log.Fatalf("export(%d): %v", n, err)
		}
		if err := setDirection(n, "out"); err != nil {
			log.Fatalf("setDirection(%d, out): %v", n, err)
		}
	}
	for _, n := range []int{
		*pinButtonSingle,
		*pinButtonDuplex,
		*pinButton3,
		*pinButton4,
	} {
		if err := export(n); err != nil {
			log.Fatalf("export(%d): %v", n, err)
		}
		if err := setDirection(n, "in"); err != nil {
			log.Fatalf("setDirection(%d, in): %v", n, err)
		}
	}

	if *useLEDs {
		/*
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
		*/
	}

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
		//Progress:  progress,
	}

	f := web.New(*tmplDir, *staticDir, &b)

	if *useButtons {
		btns, err := buttons.New(*pinButtonSingle, *pinButtonDuplex, *pinButton3, *pinButton4)
		if err != nil {
			log.Fatalf("Setting up buttons: %v", err)
		}
		btns.Backend = &b
		//btns.Progress = progress
		go btns.Run()
	}

	if *useAdafruit {
		btns, err := adafruit.New(&b)
		if err != nil {
			log.Fatalf("Setting up adafruit: %v", err)
		}
		go btns.Run()
		b.UI = btns
	}

	b.UI.Msg("IDLE", "Autoscan Ready.|Just started.")
	log.Printf("Running.")

	if *listen != "" {
		servePort(f.Mux)
	} else if *listenFCGI != "" {
		serveFCGIPort(f.Mux)
	} else {
		serveFCGI(f.Mux)
	}
}
