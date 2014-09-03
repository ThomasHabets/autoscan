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

	"github.com/ThomasHabets/autoscan/web"
)

var (
	socketPath = flag.String("socket", "", "UNIX socket to listen to.")
	listen     = flag.String("listen", ":8080", "Address to listen to.")
	configFile = flag.String("config", ".autoscan", "Config file.")
	tmplDir    = flag.String("templates", "", "Directory with HTML templates.")
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
	f := web.New(*tmplDir, d, cfg.parent)
	log.Printf("Running.")
	servePort(f.Mux)
}
