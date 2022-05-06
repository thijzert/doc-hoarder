package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strings"

	esbuild "github.com/evanw/esbuild/pkg/api"

	"github.com/thijzert/doc-hoarder/web/plumbing"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<!DOCTYPE html>\n<html><head><base href=\".\"></head><body>\n\n"))
		w.Write([]byte("<p>Hello, world</p>\n"))

		w.Write([]byte("<script src=\"assets/js/gen-link.js\"></script>\n"))

		w.Write([]byte("</body></html>"))
	})

	mux.HandleFunc("/flatten.js", func(w http.ResponseWriter, r *http.Request) {
		base_url := r.URL.Query().Get("base")
		if len(base_url) < 6 || base_url[:4] != "http" {
			w.WriteHeader(400)
			w.Write([]byte("// No base URL found"))
			return
		}

		js, err := plumbing.GetAsset("js/flatten.js")
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "%v", err)
			return
		}

		jss := string(js)

		js_base, err := json.Marshal(base_url)
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "%v", err)
			return
		}
		jss = strings.Replace(jss, "const BASE_URL = \"https://xxxxxxxxxxxxxxxxxxxxxxxx\";", fmt.Sprintf("const BASE_URL = %s;", js_base), 1)

		// TODO: generate api key
		api_key := "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"
		jss = strings.Replace(jss, "const API_KEY = \"yyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy\";", fmt.Sprintf("const API_KEY = \"%s\";", api_key), 1)

		jsb := esbuild.Transform(jss, esbuild.TransformOptions{
			MinifyWhitespace:  true,
			MinifyIdentifiers: true,
			MinifySyntax:      true,
		})
		if len(jsb.Errors) > 0 {
			w.WriteHeader(500)
			fmt.Fprintf(w, "%v", err)
			return
		}

		w.Header().Set("Content-Type", "application/javascript")
		w.Write(jsb.Code)
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
		rvm, err := json.Marshal(res)
		if err != nil {
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(rvm)
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
		rvm, err := json.Marshal(res)
		if err != nil {
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(rvm)
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

		file_path := path.Join("doc", fmt.Sprintf("g%010x", docid), "document.bin")
		f, err := os.Open(file_path)
		if err != nil {
			w.WriteHeader(404)
			fmt.Fprintf(w, "%v", err)
			return
		}

		w.Header().Set("Content-Security-Policy", "default-src: 'none'; img-src: data: 'self'; style-src: 'unsafe-inline' 'self'")
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
