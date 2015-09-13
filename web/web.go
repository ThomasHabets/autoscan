// Package web provides the Web UI for Autoscan.
package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path"

	"github.com/ThomasHabets/autoscan/backend"
	drive "google.golang.org/api/drive/v2"
)

// Frontend is a Web UI for autoscan.
type Frontend struct {
	Mux *http.ServeMux

	backend    *backend.Backend
	tmplRoot   *template.Template
	tmplScan   *template.Template
	tmplStatus *template.Template
	tmplLast   *template.Template
	staticDir  string
	drive      *drive.Service
        parent     string
}

// New creates a new Frontend.
// tmplDir is the directory that contains HTML templates.
// staticDir is the directory that contains static files, like css files, that will be accessible under /static/.
// b is the Autoscan backend.
	func New(d *drive.Service, p, tmpldir, staticDir string, b *backend.Backend) *Frontend {
	f := &Frontend{
		tmplRoot:   template.Must(template.ParseFiles(path.Join(tmpldir, "root.html"))),
		tmplScan:   template.Must(template.ParseFiles(path.Join(tmpldir, "scan.html"))),
		tmplStatus: template.Must(template.ParseFiles(path.Join(tmpldir, "status.html"))),
		tmplLast:   template.Must(template.ParseFiles(path.Join(tmpldir, "last.html"))),
		staticDir:  staticDir,
		backend:    b,
		drive:      d,
		parent: p,
		Mux:        http.NewServeMux(),
	}
	f.Mux.HandleFunc("/", f.handleRoot)
	f.Mux.HandleFunc("/scan", f.handleScan)
	f.Mux.HandleFunc("/status", f.handleStatus)
	f.Mux.HandleFunc("/last", f.handleLast)
	f.Mux.HandleFunc("/api/status", f.handleAPIStatus)
	f.Mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(f.staticDir))))
	return f
}

func (f *Frontend) handleRoot(w http.ResponseWriter, r *http.Request) {
	if len(r.URL.Path) > 1 {
		http.NotFound(w, r)
		return
	}
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
		go f.backend.Run(double)
	}
	f.tmplScan.Execute(w, &data)
}

func (f *Frontend) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	data := struct {
		State    backend.State
		LastFail error
	}{}
	data.State, data.LastFail = f.backend.Status()
	f.tmplStatus.Execute(w, &data)
}

func driveList(d *drive.Service, id, order string) ([]*drive.ChildReference, error) {
	var pageToken string
	var ret []*drive.ChildReference
        for {
                l, err := d.Children.List(id).PageToken(pageToken).OrderBy(order).Do()
                if err != nil {
			return nil, err
                }
		ret = append(ret, l.Items...)
		pageToken = l.NextPageToken
		if pageToken == "" {
			return ret, nil
		}
        }
}

func (f *Frontend) handleLast(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	type page struct {
			ThumbURL, URL string
	}
	data := struct {
		Pages []page
	}{}
	folders, err := driveList(f.drive, f.parent, "createdDate desc")
	if err != nil {
		log.Printf("Failed folder Children.List: %v\n", err)
		http.Error(w, "Internal error: listing drive files.", http.StatusInternalServerError)
		return
	}
	if len(folders) > 0 {
		images, err := driveList(f.drive, folders[0].Id, "createdDate")
		if err != nil {
			log.Printf("Failed image Children.List: %v\n", err)
			http.Error(w, "Internal error: listing drive files.", http.StatusInternalServerError)
			return
		}
		for _, i := range images {
			f, err := f.drive.Files.Get(i.Id).Fields("thumbnailLink,webContentLink,alternateLink").Do()
			if err != nil {
				log.Printf("Failed image Files.Get: %v\n", err)
				http.Error(w, "Internal error: fetching thumbnails.", http.StatusInternalServerError)
				return
			}
			data.Pages = append(data.Pages, page{
				URL:      f.AlternateLink,
				ThumbURL: f.ThumbnailLink,
			})
		}
	}

	f.tmplLast.Execute(w, &data)
}

func (f *Frontend) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	data := struct {
		State    backend.State
		LastFail string
	}{}
	var lf error
	data.State, lf = f.backend.Status()
	if lf != nil {
		data.LastFail = lf.Error()
	}
	b, err := json.Marshal(&data)
	if err != nil {
		http.Error(w, "Internal error: JSON encoding error.", http.StatusInternalServerError)
		return
	}
	w.Write(b)
}
