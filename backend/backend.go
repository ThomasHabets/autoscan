package backend

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"time"

	drive "code.google.com/p/google-api-go-client/drive/v2"

	"github.com/ThomasHabets/autoscan/backend/leds"
)

type State string

const (
	IDLE       State = "IDLE"
	SCANNING   State = "SCANNING"
	CONVERTING State = "CONVERTING"
	UPLOADING  State = "UPLOADING"
)

type Backend struct {
	// Must all be set.
	Scanimage string
	Convert   string
	ParentDir string
	Drive     *drive.Service
	Progress  chan leds.LEDMode

	// Read by non-backend, mutex protected.
	mutex    sync.Mutex
	state    State
	lastFail error
}

func (b *Backend) Init() {
	b.state = IDLE
}

func (b *Backend) scan(duplex bool, dir string) error {
	log.Printf("Starting scan. duplex=%t", duplex)

	// Start scan.
	args := []string{
		"--format", "PNM",
		"--resolution", "300",
		"--mode", "Color",
		"-b",
	}
	if duplex {
		args = append(args, "--source", "ADF Duplex")
	} else {
		args = append(args, "--source", "ADF Front")
	}
	cmd := exec.Command(b.Scanimage, args...)
	cmd.Dir = dir
	err := cmd.Run()

	// Check scan status.
	switch {
	case err == nil:
		log.Printf("That's odd, expected eventual error code 7, not 0.")
	case err.Error() == "exit status 7":
		log.Printf("Scan finished successfully.")
	default:
		return fmt.Errorf("scanning failed: %v", err)
	}
	return nil
}

func (b *Backend) convert(dir string) error {
	func() {
		b.mutex.Lock()
		defer b.mutex.Unlock()
		b.state = CONVERTING
	}()
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}
	const ext = ".pnm"
	count := 0
	for _, fn := range files {
		in := path.Join(dir, fn.Name())
		if strings.HasSuffix(in, ext) {
			count++
			out := in[0:len(in)-len(ext)] + ".jpg"
			cmd := exec.Command(b.Convert, in, out)
			cmd.Dir = dir
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			log.Printf("Running %q %q", "convert", cmd.Args)
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("running %q %q: %v. Stderr: %q", "convert", cmd.Args, err, stderr.String())
			}
			if err := os.Remove(in); err != nil {
				return fmt.Errorf("deleting pnm (%q) after convert: %v", in, err)
			}
		}
	}
	if count == 0 {
		return fmt.Errorf("zero pages scanned")
	}
	return nil
}

func (b *Backend) upload(dir string) error {
	func() {
		b.mutex.Lock()
		defer b.mutex.Unlock()
		b.state = UPLOADING
	}()

	// Get list of files.
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("readDir(%q): %v", dir, err)
	}

	// Create parent dir.
	dd, err := b.Drive.Files.Insert(&drive.File{
		Title:    time.Now().Format(time.RFC3339),
		Parents:  []*drive.ParentReference{{Id: b.ParentDir}},
		MimeType: "application/vnd.google-apps.folder",
	}).Do()
	if err != nil {
		return fmt.Errorf("creating Drive folder: %v", err)
	}

	// Upload files.
	for _, fn := range files {
		fullName := path.Join(dir, fn.Name())
		title := fmt.Sprintf("Scan %s %s", dd.Title, fn.Name())
		log.Printf("Uploading %q as %q", fn.Name(), title)

		// Upload one file.
		if err := func() error {
			inf, err := os.Open(fullName)
			if err != nil {
				return fmt.Errorf("open(%q): %v", fullName, err)
			}
			defer inf.Close()
			if _, err := b.Drive.Files.Insert(&drive.File{
				Title:       title,
				Description: fmt.Sprintf("Scanned by autoscan on %s", dd.Title),
				Parents:     []*drive.ParentReference{{Id: dd.Id}},
				MimeType:    "image/jpeg",
			}).Media(inf).Do(); err != nil {
				return fmt.Errorf("Drive.Files.Insert(): %v", err)
			}
			return nil
		}(); err != nil {
			return fmt.Errorf("uploading %q as %q: %v", fullName, title, err)
		}
	}
	return nil
}

func (b *Backend) Run(duplex bool) error {
	log.Printf("Scan run triggered in backend.")
	errout := func(err error) {
		b.mutex.Lock()
		defer b.mutex.Unlock()
		b.lastFail = err
		log.Printf("Scan run failed: %v", err)
	}

	// Setup state.
	if err := func() error {
		b.mutex.Lock()
		defer b.mutex.Unlock()

		if b.state != IDLE {
			return fmt.Errorf("state not idle, can't start scan now. state: %s", b.state)
		}

		b.state = SCANNING
		b.lastFail = nil
		return nil
	}(); err != nil {
		return err
	}
	b.Progress <- leds.GREEN
	b.Progress <- leds.BLINK

	// When done, reset to IDLE.
	defer func() {
		b.mutex.Lock()
		defer b.mutex.Unlock()
		b.state = IDLE
		if b.lastFail == nil {
			b.Progress <- leds.GREEN
		} else {
			b.Progress <- leds.RED
		}
	}()

	// Create temp dir.
	dir, err := ioutil.TempDir("", "autoscan-")
	if err != nil {
		err = fmt.Errorf("creating tempdir: %v", err)
		errout(err)
		return err
	}
	defer func() {
		log.Printf("Deleting temp dir %q", dir)
		os.RemoveAll(dir)
	}()

	if err := b.scan(duplex, dir); err != nil {
		errout(err)
		return err
	}

	// Convert.
	if err := b.convert(dir); err != nil {
		errout(err)
		return err
	}

	// Upload.
	if err := b.upload(dir); err != nil {
		errout(err)
		return err
	}

	return nil
}

func (b *Backend) Status() (State, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.state, b.lastFail
}