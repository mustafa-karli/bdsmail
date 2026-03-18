package mimeutil

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/mustafakarli/bdsmail/internal/model"
)

// AttachmentData pairs attachment metadata with its binary content.
type AttachmentData struct {
	Meta model.Attachment
	Data []byte
}

// BuildRFC822 constructs a complete RFC 5322 message.
// If attachments are provided, builds a multipart/mixed MIME message.
func BuildRFC822(from string, to []string, cc []string, subject, contentType, body, messageID string, attachments []AttachmentData) string {
	var sb strings.Builder

	// Headers
	sb.WriteString(fmt.Sprintf("From: %s\r\n", from))
	sb.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(to, ", ")))
	if len(cc) > 0 {
		sb.WriteString(fmt.Sprintf("Cc: %s\r\n", strings.Join(cc, ", ")))
	}
	sb.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	if messageID != "" {
		sb.WriteString(fmt.Sprintf("Message-ID: %s\r\n", messageID))
	}
	sb.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 -0700")))
	sb.WriteString("MIME-Version: 1.0\r\n")

	if len(attachments) == 0 {
		// Simple message
		sb.WriteString(fmt.Sprintf("Content-Type: %s; charset=UTF-8\r\n", contentType))
		sb.WriteString("\r\n")
		sb.WriteString(body)
		return sb.String()
	}

	// Multipart message with attachments
	boundary := "----=_BDSMail_" + fmt.Sprintf("%d", time.Now().UnixNano())
	sb.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n", boundary))
	sb.WriteString("\r\n")

	// Text body part
	sb.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	sb.WriteString(fmt.Sprintf("Content-Type: %s; charset=UTF-8\r\n", contentType))
	sb.WriteString("\r\n")
	sb.WriteString(body)
	sb.WriteString("\r\n")

	// Attachment parts
	for _, att := range attachments {
		sb.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		sb.WriteString(fmt.Sprintf("Content-Type: %s; name=\"%s\"\r\n", att.Meta.ContentType, att.Meta.Filename))
		sb.WriteString("Content-Transfer-Encoding: base64\r\n")
		sb.WriteString(fmt.Sprintf("Content-Disposition: attachment; filename=\"%s\"\r\n", att.Meta.Filename))
		sb.WriteString("\r\n")

		encoded := base64.StdEncoding.EncodeToString(att.Data)
		// Wrap at 76 chars per RFC 2045
		for i := 0; i < len(encoded); i += 76 {
			end := i + 76
			if end > len(encoded) {
				end = len(encoded)
			}
			sb.WriteString(encoded[i:end])
			sb.WriteString("\r\n")
		}
	}

	sb.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	return sb.String()
}

// BuildHeaders constructs RFC 5322 headers for a message.
func BuildHeaders(msg *model.Message) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("From: %s\r\n", msg.From))
	sb.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(msg.To, ", ")))
	if len(msg.CC) > 0 {
		sb.WriteString(fmt.Sprintf("Cc: %s\r\n", strings.Join(msg.CC, ", ")))
	}
	sb.WriteString(fmt.Sprintf("Subject: %s\r\n", msg.Subject))
	sb.WriteString(fmt.Sprintf("Date: %s\r\n", msg.ReceivedAt.Format("Mon, 02 Jan 2006 15:04:05 -0700")))
	if msg.MessageID != "" {
		sb.WriteString(fmt.Sprintf("Message-ID: %s\r\n", msg.MessageID))
	}
	sb.WriteString("MIME-Version: 1.0\r\n")

	if msg.HasAttachments() {
		sb.WriteString("Content-Type: multipart/mixed; boundary=\"bdsmailboundary\"\r\n")
	} else {
		sb.WriteString(fmt.Sprintf("Content-Type: %s; charset=UTF-8\r\n", msg.ContentType))
	}
	return sb.String()
}

// BuildFullMessage constructs a complete message from a model.Message + attachment data.
func BuildFullMessage(msg *model.Message, attachments []AttachmentData) string {
	return BuildRFC822(msg.From, msg.To, msg.CC, msg.Subject, msg.ContentType, msg.Body, msg.MessageID, attachments)
}
