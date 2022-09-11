package plumbing

import (
	"html/template"
	"path"
)

var localTemplates map[string]*template.Template

func init() {
	localTemplates = make(map[string]*template.Template)
}

func getTemplate(name string) (*template.Template, error) {
	if AssetsEmbedded() {
		if tp, ok := localTemplates[name]; ok {
			return tp, nil
		}
	}

	tplName := path.Base(name)

	var parentTemplate *template.Template
	var tp *template.Template

	parent := path.Dir(name)
	if parent != "." && parent != "" {
		_, err := getAsset(path.Join("templates", parent+".html"))
		if err == nil {
			parentTemplate, err = getTemplate(parent)
			if err != nil {
				return nil, err
			}
		}
	}

	b, err := getAsset(path.Join("templates", name+".html"))
	if err != nil {
		return nil, err
	}

	if parentTemplate != nil {
		tp, err = parentTemplate.Clone()
		if err != nil {
			return nil, err
		}
		tp = tp.New(tplName)
	} else {
		funcs := template.FuncMap{
			"add":     templateAddi,
			"mul":     templateMuli,
			"addf":    templateAddf,
			"mulf":    templateMulf,
			"urlfrag": template.URLQueryEscaper,
		}

		tp = template.New(tplName).Funcs(funcs)
	}

	_, err = tp.Parse(string(b))
	if err != nil {
		return nil, err
	}

	localTemplates[name] = tp
	return tp, nil
}

func templateMuli(a, b int) int {
	return a * b
}
func templateAddi(a, b int) int {
	return a + b
}
func templateMulf(a, b float64) float64 {
	return a * b
}
func templateAddf(a, b float64) float64 {
	return a + b
}

/*
package plumbing

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"path"
	"strings"

	weberrors "github.com/thijzert/speeldoos/internal/web-plumbing/errors"
	speeldoos "github.com/thijzert/speeldoos/pkg"
	"github.com/thijzert/speeldoos/pkg/search"
	"github.com/thijzert/speeldoos/pkg/web"
)

type htmlHandler struct {
	Server       *Server
	TemplateName string
	Handler      web.Handler
}

// HTMLFunc creates a HTTP handler that outputs HTML
func (s *Server) HTMLFunc(handler web.Handler, templateName string) http.Handler {
	return htmlHandler{
		Server:       s,
		TemplateName: templateName,
		Handler:      handler,
	}
}

var csp string

func init() {
}

func (h htmlHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	req, err := h.Handler.DecodeRequest(r)
	if err != nil {
		h.Error(w, r, err)
		return
	}

	tpl, err := h.Server.getTemplate(h.TemplateName)
	if err != nil {
		h.Error(w, r, err)
		return
	}

	state := h.Server.getState()
	newState, resp, err := h.Handler.HandleRequest(state, req)
	if err != nil {
		h.Error(w, r, err)
		return
	}

	err = h.Server.setState(newState)
	if err != nil {
		h.Error(w, r, err)
		return
	}

	w.Header()["Content-Type"] = []string{"text/html; charset=UTF-8"}

	csp := ""
	csp += "default-src 'self' blob: data: ; "
	csp += "script-src 'self' blob: ; "
	csp += "style-src 'self' data: 'unsafe-inline'; "
	csp += "img-src 'self' blob: data: ; "
	csp += "connect-src 'self' blob: data: ; "
	csp += "frame-src 'none' ; "
	csp += "frame-ancestors 'none'; "
	csp += "form-action 'self'; "
	w.Header()["Content-Security-Policy"] = []string{csp}
	w.Header()["X-Frame-Options"] = []string{"deny"}
	w.Header()["X-XSS-Protection"] = []string{"1; mode=block"}
	w.Header()["Referrer-Policy"] = []string{"strict-origin-when-cross-origin"}
	w.Header()["X-Content-Type-Options"] = []string{"nosniff"}

	tpData := struct {
		Version       string
		AppRoot       string
		AssetLocation string
		PageCSS       string
		Request       web.Request
		Response      web.Response
	}{
		Version:       speeldoos.PackageVersion,
		AppRoot:       h.appRoot(r),
		AssetLocation: h.appRoot(r) + "assets",
		Request:       req,
		Response:      resp,
	}

	cssTemplate := h.TemplateName
	if len(cssTemplate) > 5 && cssTemplate[0:5] == "full/" {
		cssTemplate = cssTemplate[5:]
	}

	if _, err := getAsset(path.Join("dist", "css", "pages", cssTemplate+".css")); err == nil {
		tpData.PageCSS = cssTemplate
	}

	var b bytes.Buffer
	err = tpl.ExecuteTemplate(&b, "basePage", tpData)
	if err != nil {
		h.Error(w, r, err)
		return
	}
	io.Copy(w, &b)
}



// appRoot finds the relative path to the application root
func (htmlHandler) appRoot(r *http.Request) string {
	// Find the relative path for the application root by counting the number of slashes in the relative URL
	c := strings.Count(r.URL.Path, "/") - 1
	if c == 0 {
		return "./"
	}
	return strings.Repeat("../", c)
}

func (h htmlHandler) Error(w http.ResponseWriter, r *http.Request, err error) {
	st, _ := weberrors.HTTPStatusCode(err)

	if redir, ok := err.(weberrors.Redirector); ok {
		if st == 0 {
			st = 302
		}
		h.Server.redirect(w, r, h.appRoot(r)+redir.RedirectLocation(), st)
		return
	}

	if st == 0 {
		st = 500
	}

	w.WriteHeader(st)
	fmt.Fprintf(w, "Error: %s", err)
}
*/
