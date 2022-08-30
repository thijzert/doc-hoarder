package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/thijzert/doc-hoarder/internal/storage"
	"github.com/thijzert/doc-hoarder/web/plumbing"
)

var Version string
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

	log.Printf("Doc-hoarder version %s", Version)

	docStore, err := storage.GetDocStore("")
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<!DOCTYPE html>\n<html><head><base href=\".\"></head><body>\n\n"))
		w.Write([]byte("<p>Hello, world</p>\n"))

		w.Write([]byte("<p><a href=\"ext/hoard.xpi\">Download browser extension</a></p>\n"))

		docids, err := docStore.DocumentIDs(r.Context())
		if err == nil {
			w.Write([]byte("<ul>\n"))
			for _, docid := range docids {
				fmt.Fprintf(w, "\t<li><a href=\"documents/view/g%s/\">g%s</a></li>\n", docid, docid)
			}
			w.Write([]byte("</ul>\n"))
		}

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
		type addonInfo struct {
			Updates []versionInfo `json:"updates"`
		}
		rv := struct {
			Addons map[string]addonInfo `json:"addons"`
		}{make(map[string]addonInfo)}

		addonList := []string{"hoard"}
		for _, addon := range addonList {
			rv.Addons[addon+"@"+Domain] = addonInfo{
				Updates: []versionInfo{
					versionInfo{Version, BaseURL + "ext/" + addon + ".xpi"},
				},
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

		ext, err := plumbing.GetAsset(path.Join("extensions", "_signed", extName))
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

		docid, err := docStore.NewDocumentID(r.Context())
		if err != nil {
			plumbing.JSONError(w, err)
			return
		}

		res := struct {
			ID string `json:"id"`
		}{
			ID: docid,
		}
		plumbing.WriteJSON(w, res)
	})

	draftAPI := func(r *http.Request) (storage.DocTransaction, error) {
		key := r.FormValue("api_key")
		if len(key) < 32 {
			return nil, plumbing.ErrUnauthorised
		}
		// TODO: check API key

		draft_id := strings.ToLower(r.FormValue("doc_id"))
		_, err := hex.DecodeString(draft_id)
		if err != nil || len(draft_id) != 10 {
			return nil, plumbing.BadRequest("invalid draft ID")
		}
		// TODO: check that the document has draft status, and that it's yours

		trns, err := docStore.GetDocument(draft_id)
		if err != nil {
			return nil, err
		}

		return trns, nil
	}
	mux.HandleFunc("/api/new-attachment", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")

		trns, err := draftAPI(r)
		if err != nil {
			plumbing.JSONError(w, err)
			return
		}

		ext := strings.ToLower(r.FormValue("ext"))
		if ext != "css" && ext != "svg" && ext != "png" && ext != "jpeg" {
			plumbing.JSONError(w, plumbing.BadRequest("Invalid extension '%s'", ext))
			return
		}

		attid_s, err := trns.NewAttachmentID(r.Context(), ext)
		if err != nil {
			plumbing.JSONError(w, err)
			return
		}
		attName := "t" + attid_s + "." + ext

		res := struct {
			ID       string `json:"attachment_id"`
			Filename string `json:"filename"`
		}{
			ID:       attid_s,
			Filename: "att/" + attName,
		}
		plumbing.WriteJSON(w, res)
	})

	mux.HandleFunc("/api/upload-draft", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")

		trns, err := draftAPI(r)
		if err != nil {
			plumbing.JSONError(w, err)
			return
		}

		f, _, err := r.FormFile("document")
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "%v", err)
			return
		}

		g, err := trns.WriteRootFile(r.Context(), "document.bin")
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

		trns, err := draftAPI(r)
		if err != nil {
			plumbing.JSONError(w, err)
			return
		}

		att_id := strings.ToLower(r.FormValue("att_id"))
		attName, err := storage.AttachmentNameFromID(r.Context(), trns, att_id)
		if err != nil {
			plumbing.JSONError(w, plumbing.BadRequest("invalid attachment ID"))
			return
		}

		var b bytes.Buffer
		if r.FormValue("truncate") != "1" {
			// Read current contents into the buffer - the request contents will get appended
			curr, err := trns.ReadAttachment(r.Context(), attName)
			if err == nil {
				_, err = io.Copy(&b, curr)
			}
			if err != nil {
				plumbing.JSONError(w, err)
				return
			}
			curr.Close()
		}

		f, _, err := r.FormFile("attachment")
		if err != nil {
			plumbing.JSONError(w, err)
			return
		}
		defer f.Close()

		io.Copy(&b, f)

		g, err := trns.WriteAttachment(r.Context(), attName)
		if g == nil || err != nil {
			plumbing.JSONError(w, err)
			return
		}
		defer g.Close()

		_, err = io.Copy(g, &b)
		if err != nil {
			plumbing.JSONError(w, err)
			return
		}

		res := struct {
			Message string `json:"_"`
		}{
			Message: "Chunk uploaded successfully",
		}
		plumbing.WriteJSON(w, res)
	})
	mux.HandleFunc("/api/download-attachment", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")

		trns, err := draftAPI(r)
		if err != nil {
			plumbing.JSONError(w, err)
			return
		}

		att_id := strings.ToLower(r.FormValue("att_id"))
		attName, err := storage.AttachmentNameFromID(r.Context(), trns, att_id)
		if err != nil {
			plumbing.JSONError(w, plumbing.BadRequest("invalid attachment ID"))
			return
		}

		g, err := trns.ReadAttachment(r.Context(), attName)
		if err != nil {
			plumbing.JSONError(w, err)
			return
		}
		defer g.Close()

		t := mime.TypeByExtension(path.Ext(attName))
		if t != "" {
			w.Header().Set("Content-Type", t)
		}

		io.Copy(w, g)
	})
	mux.HandleFunc("/api/proxy-attachment", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")

		trns, err := draftAPI(r)
		if err != nil {
			plumbing.JSONError(w, err)
			return
		}

		// TODO: proxy URL
		proxy_url, err := url.Parse(r.FormValue("url"))
		if err != nil || r.FormValue("url") == "" || (proxy_url.Scheme != "https" && proxy_url.Scheme != "http") {
			w.WriteHeader(400)
			fmt.Fprintf(w, "invalid url '%s'", r.FormValue("url"))
			return
		}

		pcl := &http.Client{
			Timeout: 15 * time.Second,
		}
		response, err := pcl.Get(proxy_url.String())
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "%v", err)
			return
		}
		ct := response.Header.Get("Content-Type")
		exts, err := mime.ExtensionsByType(ct)

		if ct == "" || err != nil || len(exts) == 0 {
			w.WriteHeader(400)
			plumbing.JSONMessage(w, "unknown mime type")
			return
		}
		if ct != "text/css" && !strstr(ct, "image/") && !strstr(ct, "text/css;") {
			w.WriteHeader(400)
			plumbing.JSONMessage(w, "subresource has invalid mime type '%s'", ct)
			return
		}

		ext := exts[0][1:]
		if ct == "image/jpeg" {
			// HACK: I don't like the default JPEG extension
			ext = "jpeg"
		}

		attid_s, err := trns.NewAttachmentID(r.Context(), ext)
		if err != nil {
			plumbing.JSONError(w, err)
			return
		}
		attName := "t" + attid_s + "." + ext

		f, err := trns.WriteAttachment(r.Context(), attName)
		if err != nil {
			plumbing.JSONError(w, err)
			return
		}

		_, err = io.Copy(f, response.Body)
		if err != nil {
			plumbing.JSONError(w, err)
			return
		}
		f.Close()

		res := struct {
			ID       string `json:"attachment_id"`
			Filename string `json:"filename"`
		}{
			ID:       attid_s,
			Filename: "att/" + attName,
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

		trns, err := docStore.GetDocument(fmt.Sprintf("%10x", docid))
		if err != nil {
			plumbing.HTMLError(w, err)
			return
		}

		if len(parts) >= 6 && parts[4] == "att" {
			f, err := trns.ReadAttachment(r.Context(), parts[5])
			if err != nil {
				plumbing.JSONError(w, err)
				return
			}
			defer f.Close()

			t := mime.TypeByExtension(path.Ext(parts[5]))
			if t == "text/css" || strstr(t, "image/") || strstr(t, "text/css;") {
				w.Header().Set("Content-Type", t)
				if g, ok := f.(*os.File); ok {
					fi, err := g.Stat()
					if err == nil {
						w.Header().Set("Content-Length", fmt.Sprintf("%d", fi.Size()))
					}
				}
				io.Copy(w, f)
			} else {
				w.WriteHeader(403)
				fmt.Fprintf(w, "disallowed type '%s'", t)
			}
			return
		}

		f, err := trns.ReadRootFile(r.Context(), "document.bin")
		if err != nil {
			w.WriteHeader(404)
			fmt.Fprintf(w, "%v", err)
			return
		}

		w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src data: 'self'; style-src 'unsafe-inline' 'self'")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		if g, ok := f.(*os.File); ok {
			fi, err := g.Stat()
			if err == nil {
				w.Header().Set("Content-Length", fmt.Sprintf("%d", fi.Size()))
			}
		}
		io.Copy(w, f)
	})

	listenAddr := "localhost:2690"
	log.Printf("Listening on %s", listenAddr)
	srv := &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}
	log.Fatal(srv.ListenAndServe())
}

func strstr(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return s[:len(prefix)] == prefix
}
