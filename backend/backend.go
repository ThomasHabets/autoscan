// Package backend implements the scanning, converting and uploading.
//
// The UI is outsourced to the "UI" interface, which is implemented by
// the Adafruit display, and the LED interface (well, not yet). The
// web UI polls for status via backend.Status(), currently.
//
// Triggering a scan is done by calling backend.Run().
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

	drive "google.golang.org/api/drive/v2"
)

// State describes what the backend is doing.
type State string

// The states the backend can be in. Self-explanatory.
const (
	IDLE       State = "IDLE"
	SCANNING   State = "SCANNING"
	CONVERTING State = "CONVERTING"
	UPLOADING  State = "UPLOADING"
)

// A Backend takes care of the actual scanning/converting/uploading process.
type Backend struct {
	// Must all be set.
	Scanimage string
	Convert   string
	ParentDir string
	Drive     *drive.Service
	UI        UI

	// Read by external flows, mutex protected.
	mutex    sync.Mutex
	state    State
	lastFail error
}

// UI is the physical UI for autoscan.
type UI interface {
	Msg(status, msg string)
	Run()
}

// Set the initial state of the Backend.
// Only call under mutex lock. Safe to call multiple times.
func (b *Backend) init() {
	if b.state == "" {
		b.state = IDLE
	}
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
	var stdout,stderr bytes.Buffer
	cmd := exec.Command(b.Scanimage, args...)
	cmd.Dir = dir
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	// Check scan status.
	switch {
	case err == nil:
		log.Printf("That's odd, expected eventual error code 7, not 0.")
	case err.Error() == "exit status 7":
		log.Printf("Scan finished successfully.")
	default:
		return fmt.Errorf("scanning failed: %v; stdout=%q / stderr=%q", err, stdout.String(), stderr.String())
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

	var inFiles []string
	for _, fn := range files {
		in := path.Join(dir, fn.Name())
		if strings.HasSuffix(in, ext) {
			inFiles = append(inFiles, in)
		}
	}
	sort.Slice(inFiles, func(a, b int) bool { return inFiles[a] < inFiles[b] })
	sort.SliceStable(inFiles, func(a, b int) bool { return len(inFiles[a]) < len(inFiles[b]) })
	if len(inFiles) == 0 {
		return fmt.Errorf("zero pages scanned")
	}
	cmd := exec.Command(b.Convert, inFiles...)
	// Optional: -quality
	cmd.Args = append(cmd.Args, "-compress","jpeg","out.pdf")
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	log.Printf("Running %q %q", "convert", cmd.Args)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running %q %q: %v. Stderr: %q", "convert", cmd.Args, err, stderr.String())
	}
	for _, in := range inFiles {
		if err := os.Remove(in); err != nil {
			return fmt.Errorf("deleting pnm (%q) after convert: %v", in, err)
		}
	}
	return nil
}

func (b *Backend) upload(dir string) error {
	func() {
		b.mutex.Lock()
		defer b.mutex.Unlock()
		b.state = UPLOADING
	}()

	fullName := path.Join(dir, "out.pdf")

	now := time.Now().Format(time.RFC3339)
	
	title := fmt.Sprintf("Scan %s.pdf", now)
	log.Printf("Uploading %q as %q", fullName, title)

	// Upload one file.
	inf, err := os.Open(fullName)
	if err != nil {
		return fmt.Errorf("open(%q): %v", fullName, err)
	}
	defer inf.Close()
	if _, err := b.Drive.Files.Insert(&drive.File{
		Title:       title,
		Description: fmt.Sprintf("Scanned by autoscan on %s", now),
		Parents:     []*drive.ParentReference{{Id: b.ParentDir}},
		MimeType:    "application/pdf",
	}).Media(inf).Do(); err != nil {
		return fmt.Errorf("Drive.Files.Insert(): %v", err)
	}
	return nil
}

// Run runs one scanning round (scan, convert, upload).
// If a round is already running, return error and do nothing.
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
		b.init()
		if b.state != IDLE {
			return fmt.Errorf("state not idle, can't start scan now. state: %s", b.state)
		}

		b.state = SCANNING
		b.lastFail = nil
		return nil
	}(); err != nil {
		return err
	}
	if duplex {
		b.UI.Msg("ACTIVE", "Scanning...|Double sided")
	} else {
		b.UI.Msg("ACTIVE", "Scanning...|Single sided")
	}

	// When done, reset to IDLE.
	defer func() {
		b.mutex.Lock()
		defer b.mutex.Unlock()
		b.state = IDLE
		if b.lastFail == nil {
			b.UI.Msg("IDLE", "Ready|Last scan succeeded")
		} else {
			b.UI.Msg("FAILED", "Failed!|"+b.lastFail.Error())
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
	b.UI.Msg("ACTIVE", "Converting...|")
	if err := b.convert(dir); err != nil {
		errout(err)
		return err
	}

	// Upload.
	b.UI.Msg("ACTIVE", "Uploading...|")
	if err := b.upload(dir); err != nil {
		errout(err)
		return err
	}

	return nil
}

// Status returns the state and last error of the backend.
// Both return values are valid, even if error is non-nil.
func (b *Backend) Status() (State, error) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.init()
	return b.state, b.lastFail
}
