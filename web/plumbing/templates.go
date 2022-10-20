package plumbing

import (
	"html/template"
	"path"

	"github.com/thijzert/doc-hoarder/web/plumbing/login"
	"github.com/thijzert/doc-hoarder/web/plumbing/sessions"
)

type TemplateData struct {
	AppRoot      string
	TemplateName string
	PageData     interface{}
	Session      *sessions.Session
	User         *login.User
}

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
