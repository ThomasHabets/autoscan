package web

import (
	"bytes"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"time"

	drive "code.google.com/p/google-api-go-client/drive/v2"
)

type Frontend struct {
	Mux *http.ServeMux

	tmplRoot   *template.Template
	tmplScan   *template.Template
	tmplStatus *template.Template
	backend    backend
}

type backendState string

const (
	idle       backendState = "IDLE"
	scanning   backendState = "SCANNING"
	converting backendState = "CONVERTING"
	uploading  backendState = "UPLOADING"
)

type backend struct {
	mutex     sync.Mutex
	state     backendState
	cmd       *exec.Cmd
	dir       string
	parentDir string
	lastFail  error
	drive     *drive.Service
}

func (b *backend) start(double bool) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.state != idle {
		return fmt.Errorf("state not idle, can't start scan now. state: %s", b.state)
	}
	log.Printf("Starting scan. double=%t", double)

	var err error
	b.dir, err = ioutil.TempDir("", "autoscan-web-")
	if err != nil {
		b.lastFail = fmt.Errorf("Creating tempdir: %v", err)
		return b.lastFail
	}

	args := []string{
		"--format", "PNM",
		"-d", "fujitsu",
		"--resolution", "300",
		"--mode", "Color",
		"-y", "300",
		"-x", "300",
		"-b",
		"--page-width", "300",
		"--page-height", "300",
	}
	if double {
		args = append(args, "--source", "ADF Duplex")
	} else {
		args = append(args, "--source", "ADF Front")
	}
	b.cmd = exec.Command("scanimage", args...)
	b.cmd.Dir = b.dir
	if err := b.cmd.Start(); err != nil {
		b.lastFail = fmt.Errorf("starting scan: %v", err)
		return b.lastFail
	}
	b.state = scanning
	go b.scanning()
	return nil
}

func (b *backend) scanning() {
	log.Printf("Waiting for scan to complete...")
	err := b.cmd.Wait()
	log.Printf("Scanning completed, err: %v", err)
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if err != nil && err.Error() != "exit status 7" {
		b.lastFail = fmt.Errorf("scanning failed: %v", err)
		log.Print(b.lastFail)
		b.state = idle
		return
	}
	b.state = converting
	go b.converting()
}

func (b *backend) converting() {
	log.Printf("Converting...")
	files, err := ioutil.ReadDir(b.dir)
	if err != nil {
		b.mutex.Lock()
		defer b.mutex.Unlock()
		b.lastFail = fmt.Errorf("readDir(%q): %v", b.dir, err)
		log.Print(b.lastFail)
		b.state = idle
		return
	}
	const ext = ".pnm"
	for _, fn := range files {
		in := path.Join(b.dir, fn.Name())
		if strings.HasSuffix(in, ext) {
			out := in[0:len(in)-len(ext)] + ".jpg"
			cmd := exec.Command("convert", in, out)
			cmd.Dir = b.dir
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			log.Printf("Running %q %q", "convert", cmd.Args)
			if err := cmd.Run(); err != nil {
				b.mutex.Lock()
				defer b.mutex.Unlock()
				b.lastFail = fmt.Errorf("running %q %q: %v. Stderr: %q", "convert", cmd.Args, err, stderr.String())
				log.Print(b.lastFail)
				b.state = idle
				return
			}
			if err := os.Remove(in); err != nil {
				b.mutex.Lock()
				defer b.mutex.Unlock()
				b.lastFail = fmt.Errorf("deleting pnm (%q) after convert: %v", in, err)
				log.Print(b.lastFail)
				b.state = idle
				return
			}
		}
	}
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.state = uploading
	go b.uploading()
}

func (b *backend) uploading() {
	log.Printf("Uploading...")
	files, err := ioutil.ReadDir(b.dir)
	if err != nil {
		b.mutex.Lock()
		defer b.mutex.Unlock()
		b.lastFail = fmt.Errorf("readDir(%q): %v", b.dir, err)
		log.Print(b.lastFail)
		b.state = idle
		return
	}

	dd, err := b.drive.Files.Insert(&drive.File{
		Title:    time.Now().Format(time.RFC3339),
		Parents:  []*drive.ParentReference{{Id: b.parentDir}},
		MimeType: "application/vnd.google-apps.folder",
	}).Do()
	if err != nil {
		b.mutex.Lock()
		defer b.mutex.Unlock()
		b.lastFail = fmt.Errorf("creating Drive folder: %v", err)
		log.Print(b.lastFail)
		b.state = idle
		return
	}

	for _, fn := range files {
		out := fmt.Sprintf("Scan %s %s", dd.Title, fn.Name())
		log.Printf("Uploading %q as %q", fn.Name(), out)
		if err := func() error {
			inf, err := os.Open(path.Join(b.dir, fn.Name()))
			if err != nil {
				return err
			}
			defer inf.Close()
			if _, err := b.drive.Files.Insert(&drive.File{
				Title:       out,
				Description: fmt.Sprintf("Scanned by autoscan on %s", dd.Title),
				Parents:     []*drive.ParentReference{{Id: dd.Id}},
				MimeType:    "image/jpeg",
			}).Media(inf).Do(); err != nil {
				return err
			}
			return nil
		}(); err != nil {
			b.mutex.Lock()
			defer b.mutex.Unlock()
			b.lastFail = fmt.Errorf("uploading: %v", err)
			log.Print(b.lastFail)
			b.state = idle
		}
	}
	log.Printf("Removing temp directory %q", b.dir)
	os.RemoveAll(b.dir)
	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.state = idle
	b.lastFail = nil
}

func New(tmpldir string, d *drive.Service, parent string) *Frontend {
	f := &Frontend{
		tmplRoot:   template.Must(template.ParseFiles(path.Join(tmpldir, "root.html"))),
		tmplScan:   template.Must(template.ParseFiles(path.Join(tmpldir, "scan.html"))),
		tmplStatus: template.Must(template.ParseFiles(path.Join(tmpldir, "status.html"))),
		Mux:        http.NewServeMux(),
	}
	f.backend.state = idle
	f.backend.drive = d
	f.backend.parentDir = parent
	f.Mux.HandleFunc("/", f.handleRoot)
	f.Mux.HandleFunc("/scan", f.handleScan)
	f.Mux.HandleFunc("/status", f.handleStatus)
	return f
}

func (f *Frontend) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	f.tmplRoot.Execute(w, nil)
}

func (f *Frontend) handleScan(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")

	data := struct {
		Err error
	}{}

	_, single := r.Form["single"]
	_, double := r.Form["double"]
	if double && single {
		data.Err = fmt.Errorf("both 'double' and 'single' set. Which button was pressed?")
		log.Print(data.Err)
	} else if !double && !single {
		data.Err = fmt.Errorf("neither 'double' or 'single' set. Which button was pressed?")
		log.Print(data.Err)
	} else {
		data.Err = f.backend.start(double)
	}
	f.tmplScan.Execute(w, &data)
}

func (f *Frontend) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	data := struct {
		State    string
		LastFail error
	}{}
	func() {
		f.backend.mutex.Lock()
		defer f.backend.mutex.Unlock()
		data.State = string(f.backend.state)
		data.LastFail = f.backend.lastFail
	}()
	f.tmplStatus.Execute(w, &data)
}
