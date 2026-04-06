package web

import (
	"fmt"
	"html/template"
	"net/url"
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
		"relativeTime": func(t time.Time) string {
			return formatRelativeTime(t)
		},
		"safeHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
		"humanSize": func(bytes int64) string {
			return formatHumanSize(bytes)
		},
		"urlEncode": func(s string) string {
			return url.QueryEscape(s)
		},
		"add": func(a, b int) int {
			return a + b
		},
		"sub": func(a, b int) int {
			return a - b
		},
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseGlob(templatesDir + "/*.html")
	if err != nil {
		return nil, err
	}
	return tmpl, nil
}

func formatRelativeTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		m := int(diff.Minutes())
		if m == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d min ago", m)
	case diff < 24*time.Hour:
		h := int(diff.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	case diff < 48*time.Hour:
		return "yesterday"
	case diff < 7*24*time.Hour:
		d := int(diff.Hours() / 24)
		return fmt.Sprintf("%d days ago", d)
	default:
		if t.Year() == now.Year() {
			return t.Format("Jan 02")
		}
		return t.Format("Jan 02, 2006")
	}
}

func formatHumanSize(bytes int64) string {
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%d B", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	case bytes < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
	default:
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1024*1024*1024))
	}
}
