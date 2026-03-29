package carddav

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/mustafakarli/bdsmail/internal/model"
	"github.com/mustafakarli/bdsmail/internal/store"
)

// Handler implements a minimal CardDAV server.
type Handler struct {
	store *store.Store
}

func NewHandler(s *store.Store) *Handler {
	return &Handler{store: s}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Authenticate via HTTP Basic Auth
	userEmail, ok := h.authenticate(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", `Basic realm="CardDAV"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	path := r.URL.Path

	switch r.Method {
	case "OPTIONS":
		h.handleOptions(w)
	case "PROPFIND":
		h.handlePropfind(w, r, userEmail, path)
	case http.MethodGet:
		h.handleGet(w, userEmail, path)
	case http.MethodPut:
		h.handlePut(w, r, userEmail, path)
	case http.MethodDelete:
		h.handleDelete(w, userEmail, path)
	case "REPORT":
		h.handleReport(w, r, userEmail, path)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) authenticate(r *http.Request) (string, bool) {
	username, password, ok := r.BasicAuth()
	if !ok {
		return "", false
	}
	user, err := h.store.DB.GetUserByEmail(username)
	if err != nil {
		return "", false
	}
	if !user.CheckPassword(password) {
		return "", false
	}
	return user.Email(), true
}

func (h *Handler) handleOptions(w http.ResponseWriter) {
	w.Header().Set("DAV", "1, 2, 3, addressbook")
	w.Header().Set("Allow", "OPTIONS, GET, PUT, DELETE, PROPFIND, REPORT")
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handlePropfind(w http.ResponseWriter, r *http.Request, userEmail, path string) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")

	// Determine what's being queried
	parts := parsePath(path)

	switch {
	case len(parts) == 0:
		// Root: return principal
		h.respondMultistatus(w, []davResponse{
			{Href: path, Props: []davProp{
				{Name: xml.Name{Space: "DAV:", Local: "resourcetype"}, Inner: "<d:collection/><d:principal/>"},
				{Name: xml.Name{Space: "DAV:", Local: "current-user-principal"}, Inner: "<d:href>/carddav/" + userEmail + "/</d:href>"},
				{Name: xml.Name{Space: "urn:ietf:params:xml:ns:carddav", Local: "addressbook-home-set"}, Inner: "<d:href>/carddav/" + userEmail + "/</d:href>"},
			}},
		})
	case len(parts) == 1:
		// User principal: return address book home
		h.respondMultistatus(w, []davResponse{
			{Href: path, Props: []davProp{
				{Name: xml.Name{Space: "DAV:", Local: "resourcetype"}, Inner: "<d:collection/>"},
				{Name: xml.Name{Space: "urn:ietf:params:xml:ns:carddav", Local: "addressbook-home-set"}, Inner: "<d:href>/carddav/" + userEmail + "/</d:href>"},
			}},
			{Href: "/carddav/" + userEmail + "/default/", Props: []davProp{
				{Name: xml.Name{Space: "DAV:", Local: "resourcetype"}, Inner: "<d:collection/><card:addressbook/>"},
				{Name: xml.Name{Space: "DAV:", Local: "displayname"}, Inner: "Contacts"},
			}},
		})
	case len(parts) == 2 && parts[1] == "default":
		// Address book: list contacts
		contacts, _ := h.store.DB.ListContacts(userEmail)
		responses := []davResponse{
			{Href: path, Props: []davProp{
				{Name: xml.Name{Space: "DAV:", Local: "resourcetype"}, Inner: "<d:collection/><card:addressbook/>"},
				{Name: xml.Name{Space: "DAV:", Local: "displayname"}, Inner: "Contacts"},
			}},
		}
		for _, c := range contacts {
			responses = append(responses, davResponse{
				Href: "/carddav/" + userEmail + "/default/" + c.ID + ".vcf",
				Props: []davProp{
					{Name: xml.Name{Space: "DAV:", Local: "getetag"}, Inner: `"` + c.ETag + `"`},
					{Name: xml.Name{Space: "DAV:", Local: "getcontenttype"}, Inner: "text/vcard"},
				},
			})
		}
		h.respondMultistatus(w, responses)
	case len(parts) == 3:
		// Individual contact
		contactID := strings.TrimSuffix(parts[2], ".vcf")
		c, err := h.store.DB.GetContact(contactID)
		if err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		h.respondMultistatus(w, []davResponse{
			{Href: path, Props: []davProp{
				{Name: xml.Name{Space: "DAV:", Local: "getetag"}, Inner: `"` + c.ETag + `"`},
				{Name: xml.Name{Space: "DAV:", Local: "getcontenttype"}, Inner: "text/vcard"},
			}},
		})
	}
}

func (h *Handler) handleGet(w http.ResponseWriter, userEmail, path string) {
	parts := parsePath(path)
	if len(parts) != 3 {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	contactID := strings.TrimSuffix(parts[2], ".vcf")
	c, err := h.store.DB.GetContact(contactID)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if c.OwnerEmail != userEmail {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "text/vcard; charset=utf-8")
	w.Header().Set("ETag", `"`+c.ETag+`"`)
	fmt.Fprint(w, c.VCardData)
}

func (h *Handler) handlePut(w http.ResponseWriter, r *http.Request, userEmail, path string) {
	parts := parsePath(path)
	if len(parts) != 3 {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	contactID := strings.TrimSuffix(parts[2], ".vcf")

	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	newETag := uuid.New().String()[:8]

	// Try update first
	existing, err := h.store.DB.GetContact(contactID)
	if err == nil && existing.OwnerEmail == userEmail {
		existing.VCardData = string(body)
		existing.ETag = newETag
		h.store.DB.UpdateContact(existing)
		w.Header().Set("ETag", `"`+newETag+`"`)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Create new
	c := &model.Contact{
		ID:         contactID,
		OwnerEmail: userEmail,
		VCardData:  string(body),
		ETag:       newETag,
	}
	if err := h.store.DB.CreateContact(c); err != nil {
		log.Printf("carddav: create contact error: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("ETag", `"`+newETag+`"`)
	w.WriteHeader(http.StatusCreated)
}

func (h *Handler) handleDelete(w http.ResponseWriter, userEmail, path string) {
	parts := parsePath(path)
	if len(parts) != 3 {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	contactID := strings.TrimSuffix(parts[2], ".vcf")
	c, err := h.store.DB.GetContact(contactID)
	if err != nil || c.OwnerEmail != userEmail {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	h.store.DB.DeleteContact(contactID)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleReport(w http.ResponseWriter, r *http.Request, userEmail, path string) {
	// addressbook-multiget: return requested contacts
	body, _ := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	bodyStr := string(body)

	var responses []davResponse
	// Parse href elements from the request body
	for {
		idx := strings.Index(bodyStr, "<d:href>")
		if idx < 0 {
			idx = strings.Index(bodyStr, "<D:href>")
		}
		if idx < 0 {
			break
		}
		bodyStr = bodyStr[idx+8:]
		end := strings.Index(bodyStr, "</d:href>")
		if end < 0 {
			end = strings.Index(bodyStr, "</D:href>")
		}
		if end < 0 {
			break
		}
		href := bodyStr[:end]
		bodyStr = bodyStr[end:]

		hrefParts := parsePath(href)
		if len(hrefParts) == 3 {
			contactID := strings.TrimSuffix(hrefParts[2], ".vcf")
			c, err := h.store.DB.GetContact(contactID)
			if err == nil && c.OwnerEmail == userEmail {
				responses = append(responses, davResponse{
					Href: href,
					Props: []davProp{
						{Name: xml.Name{Space: "DAV:", Local: "getetag"}, Inner: `"` + c.ETag + `"`},
						{Name: xml.Name{Space: "urn:ietf:params:xml:ns:carddav", Local: "address-data"}, Inner: xmlEscape(c.VCardData)},
					},
				})
			}
		}
	}

	h.respondMultistatus(w, responses)
}

// --- DAV XML helpers ---

type davResponse struct {
	Href  string
	Props []davProp
}

type davProp struct {
	Name  xml.Name
	Inner string
}

func (h *Handler) respondMultistatus(w http.ResponseWriter, responses []davResponse) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(207) // Multi-Status

	fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprint(w, `<d:multistatus xmlns:d="DAV:" xmlns:card="urn:ietf:params:xml:ns:carddav">`)
	for _, resp := range responses {
		fmt.Fprintf(w, `<d:response><d:href>%s</d:href><d:propstat><d:prop>`, xmlEscape(resp.Href))
		for _, p := range resp.Props {
			localName := p.Name.Local
			fmt.Fprintf(w, `<%s>%s</%s>`, xmlTagName(p.Name), p.Inner, xmlTagName(p.Name))
			_ = localName
		}
		fmt.Fprint(w, `</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`)
	}
	fmt.Fprint(w, `</d:multistatus>`)
}

func parsePath(path string) []string {
	path = strings.TrimPrefix(path, "/carddav/")
	path = strings.TrimPrefix(path, "/carddav")
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func xmlTagName(name xml.Name) string {
	if strings.HasPrefix(name.Space, "DAV:") {
		return "d:" + name.Local
	}
	if strings.Contains(name.Space, "carddav") {
		return "card:" + name.Local
	}
	return name.Local
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
