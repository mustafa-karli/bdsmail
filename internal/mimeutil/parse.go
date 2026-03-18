package mimeutil

import (
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"

	"github.com/mustafakarli/bdsmail/internal/model"
)

// ParsedEmail holds the result of parsing a MIME email.
type ParsedEmail struct {
	TextBody    string
	HTMLBody    string
	ContentType string // "text/plain" or "text/html"
	Attachments []ParsedAttachment
}

// ParsedAttachment is a raw attachment extracted from a MIME message.
type ParsedAttachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

// Parse extracts body text and attachments from a parsed mail.Message.
// For non-multipart messages, returns the body as-is with no attachments.
func Parse(msg *mail.Message) (*ParsedEmail, error) {
	ct := msg.Header.Get("Content-Type")
	if ct == "" {
		ct = "text/plain"
	}

	mediaType, params, err := mime.ParseMediaType(ct)
	if err != nil {
		// Unparseable content-type, treat as plain text
		body, _ := io.ReadAll(msg.Body)
		return &ParsedEmail{TextBody: string(body), ContentType: "text/plain"}, nil
	}

	// Simple non-multipart message
	if !strings.HasPrefix(mediaType, "multipart/") {
		body, _ := io.ReadAll(msg.Body)
		contentType := "text/plain"
		if strings.Contains(mediaType, "html") {
			contentType = "text/html"
		}
		return &ParsedEmail{TextBody: string(body), ContentType: contentType}, nil
	}

	// Multipart message
	boundary := params["boundary"]
	if boundary == "" {
		body, _ := io.ReadAll(msg.Body)
		return &ParsedEmail{TextBody: string(body), ContentType: "text/plain"}, nil
	}

	result := &ParsedEmail{ContentType: "text/plain"}
	reader := multipart.NewReader(msg.Body, boundary)

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}

		partCT := part.Header.Get("Content-Type")
		partMediaType, _, _ := mime.ParseMediaType(partCT)
		disposition := part.Header.Get("Content-Disposition")

		data, readErr := io.ReadAll(part)
		if readErr != nil {
			continue
		}

		// Decode base64 if Content-Transfer-Encoding is base64
		encoding := part.Header.Get("Content-Transfer-Encoding")
		if strings.EqualFold(encoding, "base64") {
			decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
			if err == nil {
				data = decoded
			}
		}

		// Check if this is an attachment
		if strings.HasPrefix(disposition, "attachment") || (disposition == "" && !strings.HasPrefix(partMediaType, "text/")) {
			filename := part.FileName()
			if filename == "" {
				_, dParams, _ := mime.ParseMediaType(disposition)
				filename = dParams["filename"]
			}
			if filename == "" {
				filename = fmt.Sprintf("attachment_%d", len(result.Attachments)+1)
			}

			if partMediaType == "" {
				partMediaType = "application/octet-stream"
			}

			result.Attachments = append(result.Attachments, ParsedAttachment{
				Filename:    filename,
				ContentType: partMediaType,
				Data:        data,
			})
			continue
		}

		// Text part
		if strings.HasPrefix(partMediaType, "text/html") {
			result.HTMLBody = string(data)
			result.ContentType = "text/html"
		} else if strings.HasPrefix(partMediaType, "text/plain") {
			result.TextBody = string(data)
		}
	}

	// Prefer HTML body if available
	if result.HTMLBody != "" && result.TextBody == "" {
		result.TextBody = result.HTMLBody
	}

	return result, nil
}

// ToAttachmentMeta converts parsed attachments to model metadata (without data).
func ToAttachmentMeta(parsed []ParsedAttachment, keyPrefix string) []model.Attachment {
	var result []model.Attachment
	for i, pa := range parsed {
		result = append(result, model.Attachment{
			ID:          fmt.Sprintf("%s_%d", keyPrefix, i),
			Filename:    pa.Filename,
			ContentType: pa.ContentType,
			Size:        int64(len(pa.Data)),
			BucketKey:   fmt.Sprintf("%s/attach_%d_%s", keyPrefix, i, pa.Filename),
		})
	}
	return result
}
