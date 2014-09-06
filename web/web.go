package web

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path"

	"github.com/ThomasHabets/autoscan/backend"
)

type Frontend struct {
	Mux     *http.ServeMux
	Backend *backend.Backend

	tmplRoot   *template.Template
	tmplScan   *template.Template
	tmplStatus *template.Template
	staticDir  string
}

func New(tmpldir, staticDir string) *Frontend {
	f := &Frontend{
		tmplRoot:   template.Must(template.ParseFiles(path.Join(tmpldir, "root.html"))),
		tmplScan:   template.Must(template.ParseFiles(path.Join(tmpldir, "scan.html"))),
		tmplStatus: template.Must(template.ParseFiles(path.Join(tmpldir, "status.html"))),
		staticDir:  staticDir,
		Mux:        http.NewServeMux(),
	}
	f.Mux.HandleFunc("/", f.handleRoot)
	f.Mux.HandleFunc("/scan", f.handleScan)
	f.Mux.HandleFunc("/status", f.handleStatus)
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
		go f.Backend.Run(double)
	}
	if data.Err != nil {
		f.tmplScan.Execute(w, &data)
	} else {
		http.Redirect(w, r, "/status", http.StatusFound)
	}
}

func (f *Frontend) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=UTF-8")
	data := struct {
		State    backend.State
		LastFail error
	}{}
	data.State, data.LastFail = f.Backend.Status()
	f.tmplStatus.Execute(w, &data)
}
