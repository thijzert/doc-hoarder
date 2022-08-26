package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/thijzert/doc-hoarder/web/plumbing"
)

var BaseURL string
var Domain string

func main() {
	if BaseURL == "" {
		log.Fatal("baseURL not compiled in")
	}
	u, err := url.Parse(BaseURL)
	if err == nil {
		Domain = u.Host
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<!DOCTYPE html>\n<html><head><base href=\".\"></head><body>\n\n"))
		w.Write([]byte("<p>Hello, world</p>\n"))

		w.Write([]byte("<script src=\"assets/js/gen-link.js\"></script>\n"))

		w.Write([]byte("</body></html>"))
	})

	mux.HandleFunc("/assets/js/", func(w http.ResponseWriter, r *http.Request) {
		jspath := strings.Replace(r.URL.Path, "./", "-", -1)
		if len(jspath) < 12 {
			w.WriteHeader(404)
			return
		}

		js, err := plumbing.GetAsset(jspath[8:])
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "%v", err)
			return
		}

		w.Header().Set("Content-Type", "application/javascript")
		w.Write([]byte(js))
	})

	mux.HandleFunc("/ext/updates.json", func(w http.ResponseWriter, r *http.Request) {
		type versionInfo struct {
			Version    string `json:"version"`
			UpdateLink string `json:"update_link"`
		}
		rv := struct {
			Addons map[string][]versionInfo `json:"addons"`
		}{make(map[string][]versionInfo)}

		addonList := []string{"hoard"}
		for _, addon := range addonList {
			rv.Addons[addon+"@"+Domain] = []versionInfo{
				versionInfo{"0.1", BaseURL + "ext/" + addon + ".xpi"},
			}
		}

		plumbing.WriteJSON(w, rv)
	})
	mux.HandleFunc("/ext/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		extName := parts[2]
		if len(extName) < 5 || extName[0] == '.' || extName[len(extName)-4:] != ".xpi" {
			w.WriteHeader(400)
			return
		}

		ext, err := plumbing.GetAsset(path.Join("extensions", extName[:len(extName)-4]+"-signed.xpi"))
		if err != nil {
			ext, err = plumbing.GetAsset(path.Join("extensions", extName))
		}
		if err != nil {
			w.WriteHeader(404)
			plumbing.WriteJSON(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/x-xpinstall")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(ext)))
		w.Write(ext)
	})

	mux.HandleFunc("/api/new-doc", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")

		key := r.FormValue("api_key")
		if len(key) < 32 {
			w.WriteHeader(401)
			return
		}
		// TODO: check API key

		var b []byte = make([]byte, 5)
		rand.Read(b)

		// TODO: check for collisions

		res := struct {
			ID string `json:"id"`
		}{
			ID: hex.EncodeToString(b),
		}
		plumbing.WriteJSON(w, res)
	})
	mux.HandleFunc("/api/new-attachment", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")

		key := r.FormValue("api_key")
		if len(key) < 32 {
			w.WriteHeader(401)
			return
		}
		// TODO: check API key

		draft_id := strings.ToLower(r.FormValue("doc_id"))
		_, err := hex.DecodeString(draft_id)
		if err != nil || len(draft_id) != 10 {
			w.WriteHeader(400)
			fmt.Fprintf(w, "doc_id: '%s' -> %v", draft_id, err)
			return
		}
		// TODO: check that the document has draft status, and that it's yours

		ext := strings.ToLower(r.FormValue("ext"))
		if ext != "css" && ext != "svg" && ext != "png" && ext != "jpeg" {
			w.WriteHeader(400)
			fmt.Fprintf(w, "Invalid extension '%s'", ext)
			return
		}
		// TODO: check that the document has draft status, and that it's yours

		// Generate new attachment ID
		var b []byte = make([]byte, 5)
		rand.Read(b)
		attid_s := hex.EncodeToString(b)

		// TODO: check for collisions

		os.MkdirAll(path.Join("doc", "g"+draft_id, "att"), 0755)

		f, err := os.Create(path.Join("doc", "g"+draft_id, "att", "t"+attid_s+"."+ext))
		defer f.Close()
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "%v", err)
			return
		}

		res := struct {
			ID string `json:"attachment_id"`
		}{
			ID: attid_s,
		}
		plumbing.WriteJSON(w, res)
	})

	mux.HandleFunc("/api/upload-draft", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")

		key := r.FormValue("api_key")
		if len(key) < 32 {
			w.WriteHeader(401)
			return
		}
		// TODO: check API key

		draft_id := strings.ToLower(r.FormValue("doc_id"))
		_, err := hex.DecodeString(draft_id)
		if err != nil || len(draft_id) != 10 {
			w.WriteHeader(400)
			fmt.Fprintf(w, "doc_id: '%s' -> %v", draft_id, err)
			return
		}
		// TODO: check that the document has draft status, and that it's yours

		f, _, err := r.FormFile("document")
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "%v", err)
			return
		}

		err = os.MkdirAll(path.Join("doc", "g"+draft_id), 0755)
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "%v", err)
			return
		}

		g, err := os.OpenFile(path.Join("doc", "g"+draft_id, "document.bin"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "%v", err)
			return
		}
		defer g.Close()

		_, err = io.Copy(g, f)
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "%v", err)
			return
		}

		res := struct {
			Message string `json:"_"`
		}{
			Message: "Chunk uploaded successfully",
		}
		plumbing.WriteJSON(w, res)
	})
	mux.HandleFunc("/api/upload-attachment", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")

		key := r.FormValue("api_key")
		if len(key) < 32 {
			w.WriteHeader(401)
			return
		}
		// TODO: check API key

		draft_id := strings.ToLower(r.FormValue("doc_id"))
		_, err := hex.DecodeString(draft_id)
		if err != nil || len(draft_id) != 10 {
			w.WriteHeader(400)
			fmt.Fprintf(w, "doc_id: '%s' -> %v", draft_id, err)
			return
		}
		// TODO: check that the document has draft status, and that it's yours

		att_id := strings.ToLower(r.FormValue("att_id"))
		_, err = hex.DecodeString(att_id)
		if err != nil || len(att_id) != 10 {
			w.WriteHeader(400)
			fmt.Fprintf(w, "att_id: '%s' -> %v", att_id, err)
			return
		}
		// TODO: check that this attachment ID was created beforehand

		f, _, err := r.FormFile("attachment")
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "%v", err)
			return
		}

		err = os.MkdirAll(path.Join("doc", "g"+draft_id, "att"), 0755)
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "%v", err)
			return
		}

		var g *os.File
		for _, ext := range []string{"css", "svg", "png", "jpeg", "svg"} {
			g, err = os.OpenFile(path.Join("doc", "g"+draft_id, "att", fmt.Sprintf("t%s.%s", att_id, ext)), os.O_APPEND|os.O_WRONLY, 0644)
			if err == nil {
				break
			}
		}

		if g == nil || err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "%v", err)
			return
		}
		defer g.Close()

		_, err = io.Copy(g, f)
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "%v", err)
			return
		}

		res := struct {
			Message string `json:"_"`
		}{
			Message: "Chunk uploaded successfully",
		}
		plumbing.WriteJSON(w, res)
	})

	mux.HandleFunc("/documents/view/", func(w http.ResponseWriter, r *http.Request) {
		var docid int64
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) <= 3 {
			fmt.Fprintf(w, "Parts: %v", parts)
			w.WriteHeader(404)
			return
		}

		_, err := fmt.Sscanf(parts[3], "g%010x", &docid)
		if err != nil {
			w.WriteHeader(404)
			fmt.Fprintf(w, "%v", err)
			return
		}

		if len(parts) == 4 {
			w.Header().Set("Location", fmt.Sprintf("g%010x/", docid))
			w.WriteHeader(302)
			return
		}

		if len(parts) >= 6 && parts[4] == "att" {
			var attid int64
			var ext string
			_, err := fmt.Sscanf(parts[5], "t%010x.%s", &attid, &ext)
			if err != nil || strings.ContainsAny(parts[5], "/ \n\r\x00") {
				w.WriteHeader(404)
				fmt.Fprintf(w, "%v", err)
				return
			}

			file_path := path.Join("doc", fmt.Sprintf("g%010x", docid), "att", parts[5])
			f, err := os.Open(file_path)
			if err != nil {
				w.WriteHeader(404)
				fmt.Fprintf(w, "%v", err)
				return
			}

			if ext == "css" {
				w.Header().Set("Content-Type", "text/css")
			} else if ext == "svg" {
				w.Header().Set("Content-Type", "image/svg+xml")
			} else if ext == "png" || ext == "jpeg" {
				w.Header().Set("Content-Type", "image/"+ext)
			} else {
				w.WriteHeader(403)
				fmt.Fprintf(w, "%v", err)
				return
			}

			fi, err := f.Stat()
			if err == nil {
				w.Header().Set("Content-Length", fmt.Sprintf("%d", fi.Size()))
			}
			io.Copy(w, f)
			return
		}

		file_path := path.Join("doc", fmt.Sprintf("g%010x", docid), "document.bin")
		f, err := os.Open(file_path)
		if err != nil {
			w.WriteHeader(404)
			fmt.Fprintf(w, "%v", err)
			return
		}

		w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src data: 'self'; style-src 'unsafe-inline' 'self'")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		fi, err := f.Stat()
		if err == nil {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", fi.Size()))
		}
		io.Copy(w, f)
	})

	srv := &http.Server{
		Addr:    "localhost:2690",
		Handler: mux,
	}
	log.Fatal(srv.ListenAndServe())
}
