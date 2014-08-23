package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"sync"

	"code.google.com/p/goauth2/oauth"
	drive "code.google.com/p/google-api-go-client/drive/v2"
)

const (
	spaces = "\n\t\r "
)

var (
	progressRE = regexp.MustCompile(`^Progress: [0-9.%]+$`)

	configure  = flag.Bool("configure", false, "Configure autoscan.")
	configFile = flag.String("config", ".autoscan", "Config file.")

	scanImage = flag.String("cmd-scanimage", "scanimage", "Location of scanimage binary.")
	converter = flag.String("cmd-convert", "convert", "Location of 'convert' binary.")

	scanner    = flag.String("scanner", "", "Scan device.")
	resolution = flag.String("resolution", "300", "Scan resolution.")
	feeder     = flag.Bool("feeder", false, "Scanner is a paper feeder.")

	name = flag.String("name", "", "Name of uploaded files.")
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

func auth(id, secret string) (string, error) {
	cfg := oauthConfig(id, secret)
	fmt.Printf("Cut and paste this URL into your browser:\n  %s\n", cfg.AuthCodeURL(""))
	fmt.Printf("Returned code: ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	t := oauth.Transport{Config: cfg}
	token, err := t.Exchange(line)
	if err != nil {
		return "", err
	}
	return token.RefreshToken, nil
}

func doAuth() error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Printf("ParentID: ")
	parent, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	parent = strings.Trim(parent, spaces)

	fmt.Printf("ClientID: ")
	id, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	id = strings.Trim(id, spaces)

	fmt.Printf("ClientSecret: ")
	secret, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	secret = strings.Trim(secret, spaces)

	token, err := auth(id, secret)
	if err != nil {
		return err
	}
	f, err := os.Create(*configFile)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(f, "%s\n%s\n%s\n%s\n", id, secret, token, parent)
	return nil
}

func scanArgs() []string {
	args := []string{
		"--format", "PNM",
		"--resolution", *resolution,
		"-p",
	}
	if *scanner != "" {
		args = append(args, "--device-name", *scanner)
	}
	return args
}

func scanBatch(dir string) error {
	args := append(scanArgs(), "-batch")
	log.Printf("Running %q %v\n", *scanImage, args)
	cmd := exec.Command(*scanImage, args...)
	cmd.Dir = dir
	cmd.Stderr = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func printProgress(r io.Reader) string {
	reader := bufio.NewReader(r)
	var buf bytes.Buffer
	var err error
	for {
		var line string
		line, err = reader.ReadString('\r')
		if err != nil {
			break
		}
		p := strings.Trim(line, "\r")
		if progressRE.MatchString(p) {
			fmt.Printf("\r%s", p)
		}
		fmt.Fprint(&buf, line)
	}
	fmt.Println()
	if err != io.EOF {
		log.Printf("Error reading scanimage stderr: %v", err)
	}
	return buf.String()
}

func scanManualSingle(dir string, page int) error {
	of, err := os.Create(path.Join(dir, fmt.Sprintf("scan-%05d.pnm", page)))
	if err != nil {
		return err
	}
	defer of.Close()
	args := append(scanArgs())

	//log.Printf("Running %q %v\n", *scanImage, args)
	cmd := exec.Command(*scanImage, args...)
	cmd.Dir = dir
	cmd.Stdout = of
	errPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting scanimage (%q %q): %v", *scanImage, args, err)
	}
	var wg sync.WaitGroup
	var stderr string
	wg.Add(1)
	go func() {
		defer wg.Done()
		stderr = printProgress(errPipe)
	}()
	err = cmd.Wait()
	wg.Wait()
	if err != nil {
		return fmt.Errorf("scanimage subprocess (%q %q): %v. Stderr: %q", *scanImage, args, err, stderr)
	}
	// TODO: possibly convert in a goroutine.
	if err := convert(dir); err != nil {
		log.Fatalf("Converting: %v", err)
	}
	return nil
}

func scanManual(dir string) error {
	n := 1
	for {
		fmt.Printf("Scan? [Y/n] ")
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil
		}
		line = strings.Trim(line, "\n\r\t ")
		switch line {
		case "n":
			return nil
		case "":
			if err := scanManualSingle(dir, n); err != nil {
				fmt.Printf("Error scanning: %v\n", err)
			} else {
				n++
			}
		default:
			fmt.Printf("Unknown choice %q\n", line)
			continue
		}
	}
	return nil
}

func convert(dir string) error {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}
	const ext = ".pnm"
	for _, fn := range files {
		in := path.Join(dir, fn.Name())
		if strings.HasSuffix(in, ext) {
			out := in[0:len(in)-len(ext)] + ".jpg"
			cmd := exec.Command(*converter, in, out)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			//log.Printf("Running %q %q", *converter, cmd.Args)
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("running %q %q: %v. Stderr: %q", *converter, cmd.Args, err, stderr.String())
			}
			if err := os.Remove(in); err != nil {
				return fmt.Errorf("deleting pnm (%q) after convert: %v", in, err)
			}
		}
	}
	return nil
}
func upload(cfg *config, d *drive.Service, dir string) error {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}
	dd, err := d.Files.Insert(&drive.File{
		Title: *name,
		Parents: []*drive.ParentReference{
			{Id: cfg.parent},
		},
		MimeType: "application/vnd.google-apps.folder",
	}).Do()
	if err != nil {
		return fmt.Errorf("creating folder %q: %v", *name, err)
	}

	for _, fn := range files {
		err := func() error {
			in := path.Join(dir, fn.Name())
			out := fmt.Sprintf("%s %s", *name, fn.Name())
			log.Printf("Uploading %q as %q", fn.Name(), out)
			inf, err := os.Open(in)
			if err != nil {
				return err
			}
			defer inf.Close()
			if _, err := d.Files.Insert(&drive.File{
				Title: out,
				//Description: "",
				Parents: []*drive.ParentReference{
					{Id: dd.Id},
				},
				MimeType: "image/jpeg",
			}).Media(inf).Do(); err != nil {
				return err
			}
			return nil
		}()
		if err != nil {
			return err
		}
	}
	return nil
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

	if *name == "" {
		log.Fatal("-name must be supplied")
	}

	if *configure {
		if err := doAuth(); err != nil {
			log.Fatalf("doAuth: %v", err)
		}
		return
	}

	cfg, err := readConfig()
	if err != nil {
		log.Fatalf("Reading config: %v", err)
	}

	t, err := connect(cfg.clientID, cfg.clientSecret, cfg.refreshToken)
	if err != nil {
		log.Fatalf("Connecting to Google Drive: %v", err)
	}
	d, err := drive.New(t.Client())
	if err != nil {
		log.Fatalf("Creating Google Drive client: %v", err)
	}

	dir, err := ioutil.TempDir("", "autoscan-")
	if err != nil {
		log.Fatalf("Creating tempdir: %v", err)
	}
	defer os.RemoveAll(dir)
	//log.Printf("Storing scanned files in %q\n", dir)
	if *feeder {
		if err := scanBatch(dir); err != nil {
			log.Fatalf("error scanning: %v", err)
		}
	} else {
		if err := scanManual(dir); err != nil {
			log.Fatalf("error scanning: %v", err)
		}
	}
	log.Printf("Converting...")
	if err := convert(dir); err != nil {
		log.Fatalf("Converting: %v", err)
	}

	log.Printf("Uploading...")
	if err := upload(cfg, d, dir); err != nil {
		log.Fatalf("Uploading: %v", err)
	}
}
