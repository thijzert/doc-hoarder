package plumbing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"path"
	"strings"
)

type Handler interface {
	Handle(r *http.Request) (interface{}, error)
}

type HandlerFunc func(r *http.Request) (interface{}, error)

func (f HandlerFunc) Handle(r *http.Request) (interface{}, error) {
	return f(r)
}

func AsJSON(h Handler) http.Handler {
	return jsonHandler{
		Handler: h,
	}
}

type jsonHandler struct {
	Handler Handler
}

func (j jsonHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	type jsonErr struct {
		Headline string `json:"error"`
		Message  string `json:"_"`
	}

	code := 200

	rv, err := j.Handler.Handle(r)
	if err != nil {
		code = 500
		if sc, ok := err.(HTTPStatuser); ok {
			code = sc.StatusCode()
		}
		if red, ok := err.(httpRedirect); ok {
			w.Header().Set("Location", red.Location)
		}

		rrv := jsonErr{"internal server error", "an internal error has occurred"}
		if um, ok := err.(UserMessager); ok {
			rrv.Headline, rrv.Message = um.ErrorMessage()
		}

		rv = rrv
	}

	if bl, ok := rv.(Blob); ok {
		bl.WriteTo(w)
		return
	}

	rvm, err := json.Marshal(rv)
	if err != nil {
		code = 500
		rvm, _ = json.Marshal(jsonErr{"error encoding to json", err.Error()})
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(rvm)))
	w.WriteHeader(code)
	w.Write(rvm)
}

func AsHTML(h Handler, templateName string) http.Handler {
	return htmlHandler{
		Handler:      h,
		TemplateName: templateName,
	}
}

type Blob struct {
	ContentType string
	Contents    []byte
	Header      http.Header
}

func (bl Blob) WriteTo(w http.ResponseWriter) {
	w.Header().Set("Content-Type", bl.ContentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(bl.Contents)))
	if bl.Header != nil {
		for k, vs := range bl.Header {
			w.Header()[k] = vs
		}
	}
	w.Write(bl.Contents)
}

type htmlHandler struct {
	Handler      Handler
	TemplateName string
}

func (h htmlHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tpData := struct {
		AppRoot      string
		TemplateName string
		PageData     interface{}
	}{
		AppRoot:      appRoot(r),
		TemplateName: path.Base(h.TemplateName),
	}

	rootName := strings.SplitN(h.TemplateName, "/", 2)[0]

	tpl, err := getTemplate(h.TemplateName)
	if err == nil {
		tpData.PageData, err = h.Handler.Handle(r)
	}

	if err != nil {
		code := 500
		if sc, ok := err.(HTTPStatuser); ok {
			code = sc.StatusCode()
		}

		if red, ok := err.(httpRedirect); ok {
			w.Header().Set("Location", red.Location)
		}

		npd := struct {
			Headline string
			Message  string
		}{"internal server error", "an internal error has occurred"}

		if um, ok := err.(UserMessager); ok {
			npd.Headline, npd.Message = um.ErrorMessage()
		}
		tpData.PageData = npd

		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(code)

		tpl, err := getTemplate("page/error")

		var b bytes.Buffer
		if err == nil {
			err = tpl.ExecuteTemplate(&b, rootName, tpData)
			if err == nil {
				io.Copy(w, &b)
				return
			}
		}

		fmt.Fprintf(w, "<!DOCTYPE html>\n<html><head><base href=\"%s\"></head><body>\n\n", html.EscapeString(tpData.AppRoot))
		fmt.Fprintf(w, "<h1>%s</h1>", html.EscapeString(npd.Headline))
		fmt.Fprintf(w, "<p>%s</p>", html.EscapeString(npd.Message))
		fmt.Fprintf(w, "</body></html>")
		return
	}

	if bl, ok := tpData.PageData.(Blob); ok {
		bl.WriteTo(w)
		return
	}

	w.Header().Set("Content-Type", "text/html")

	var b bytes.Buffer
	err = tpl.ExecuteTemplate(&b, rootName, tpData)
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "<!DOCTYPE html>\n<html><head><base href=\"%s\"></head><body>\n\n", html.EscapeString(tpData.AppRoot))
		fmt.Fprintf(w, "<h1>internal server error</h1>")
		fmt.Fprintf(w, "<p>An error occurred while displaying this page to you.</p>")
		if IsDevBuild() {
			fmt.Fprintf(w, "<section>%s</section>", html.EscapeString(err.Error()))
		}
		fmt.Fprintf(w, "</body></html>")
		return
	}

	io.Copy(w, &b)
}

func appRoot(r *http.Request) string {
	// TODO
	return "."
}

type Headerer interface {
	HTTPHeader() http.Header
}
type Unwrapper interface {
	UnwrapResult() interface{}
}

func CORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		h.ServeHTTP(w, r)
	})
}
