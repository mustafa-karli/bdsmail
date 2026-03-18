package web

import (
	"html/template"
	"strings"
	"time"
)

func loadTemplates(templatesDir string) (*template.Template, error) {
	funcMap := template.FuncMap{
		"joinAddrs": func(addrs []string) string {
			return strings.Join(addrs, ", ")
		},
		"formatTime": func(t time.Time) string {
			return t.Format("Jan 02, 2006 3:04 PM")
		},
		"safeHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseGlob(templatesDir + "/*.html")
	if err != nil {
		return nil, err
	}
	return tmpl, nil
}
