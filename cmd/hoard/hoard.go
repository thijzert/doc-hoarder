package main

import (
	"fmt"
	"html"
	"log"
	"net/http"

	esbuild "github.com/evanw/esbuild/pkg/api"

	"github.com/thijzert/doc-hoarder/web/plumbing"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<p>Hello, world</p>"))

		js, err := plumbing.GetAsset("js/flatten.js")
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "%v", err)
			return
		}
		jsb := esbuild.Transform(string(js), esbuild.TransformOptions{
			MinifyWhitespace:  true,
			MinifyIdentifiers: true,
			MinifySyntax:      true,
		})
		if len(jsb.Errors) > 0 {
			w.WriteHeader(500)
			fmt.Fprintf(w, "%v", err)
			return
		}

		codestr := html.EscapeString(string(jsb.Code))

		fmt.Fprintf(w, "<p><a onclick=\"return false;\" href=\"javascript:%s\">HOARD</a> - %d bytes</p>", codestr, len(codestr))
	})

	mux.HandleFunc("/flatten.js", func(w http.ResponseWriter, r *http.Request) {
		js, err := plumbing.GetAsset("js/flatten.js")
		if err != nil {
			w.WriteHeader(500)
			fmt.Fprintf(w, "%v", err)
			return
		}

		w.Header().Set("Content-Type", "application/javascript")
		w.Write(js)
	})

	srv := &http.Server{
		Addr:    "localhost:2690",
		Handler: mux,
	}
	log.Fatal(srv.ListenAndServe())
}
