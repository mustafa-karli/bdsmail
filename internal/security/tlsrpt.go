package security

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// TLSReporter collects TLS connection results and generates RFC 8460 reports.
type TLSReporter struct {
	mu           sync.Mutex
	events       map[string]*domainTLSEvents // recipient domain -> events
	interval     time.Duration
	senderDomain string
	sendFunc     func(from string, to []string, subject, contentType, body, messageID string) error
}

type domainTLSEvents struct {
	SuccessCount int
	FailureCount int
	Failures     []TLSFailureEvent
}

// TLSFailureEvent records a TLS connection failure.
type TLSFailureEvent struct {
	ResultType    string    // e.g. "starttls-not-supported", "certificate-expired", "certificate-host-mismatch", "validation-failure"
	SendingMTA    string
	ReceivingMX   string
	FailureReason string
	Timestamp     time.Time
}

func NewTLSReporter(senderDomain string, interval time.Duration) *TLSReporter {
	r := &TLSReporter{
		events:       make(map[string]*domainTLSEvents),
		interval:     interval,
		senderDomain: senderDomain,
	}
	go r.reportLoop()
	return r
}

// SetSendFunc sets the function used to send report emails.
// This breaks the circular dependency between reporter and relay.
func (t *TLSReporter) SetSendFunc(fn func(from string, to []string, subject, contentType, body, messageID string) error) {
	t.mu.Lock()
	t.sendFunc = fn
	t.mu.Unlock()
}

// RecordSuccess records a successful TLS connection to a recipient domain.
func (t *TLSReporter) RecordSuccess(recipientDomain, mx string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	ev, ok := t.events[recipientDomain]
	if !ok {
		ev = &domainTLSEvents{}
		t.events[recipientDomain] = ev
	}
	ev.SuccessCount++
}

// RecordFailure records a TLS connection failure.
func (t *TLSReporter) RecordFailure(recipientDomain string, event TLSFailureEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	ev, ok := t.events[recipientDomain]
	if !ok {
		ev = &domainTLSEvents{}
		t.events[recipientDomain] = ev
	}
	ev.FailureCount++
	ev.Failures = append(ev.Failures, event)
}

func (t *TLSReporter) reportLoop() {
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()
	for range ticker.C {
		t.generateAndSendReports()
	}
}

func (t *TLSReporter) generateAndSendReports() {
	t.mu.Lock()
	events := t.events
	t.events = make(map[string]*domainTLSEvents)
	sendFunc := t.sendFunc
	t.mu.Unlock()

	if sendFunc == nil || len(events) == 0 {
		return
	}

	for domain, ev := range events {
		reportAddr := lookupTLSRPT(domain)
		if reportAddr == "" {
			continue
		}

		report := t.buildReport(domain, ev)
		reportJSON, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			log.Printf("TLSRPT: failed to marshal report for %s: %v", domain, err)
			continue
		}

		from := "tlsrpt@" + t.senderDomain
		subject := fmt.Sprintf("Report Domain: %s Submitter: %s", domain, t.senderDomain)

		err = sendFunc(from, []string{reportAddr}, subject, "application/tlsrpt+json", string(reportJSON), "")
		if err != nil {
			log.Printf("TLSRPT: failed to send report for %s to %s: %v", domain, reportAddr, err)
		} else {
			log.Printf("TLSRPT: sent report for %s to %s (success=%d, failure=%d)", domain, reportAddr, ev.SuccessCount, ev.FailureCount)
		}
	}
}

// tlsrptReport is the RFC 8460 JSON report structure.
type tlsrptReport struct {
	OrganizationName string            `json:"organization-name"`
	DateRange        tlsrptDateRange   `json:"date-range"`
	ContactInfo      string            `json:"contact-info"`
	ReportID         string            `json:"report-id"`
	Policies         []tlsrptPolicy    `json:"policies"`
}

type tlsrptDateRange struct {
	StartDatetime string `json:"start-datetime"`
	EndDatetime   string `json:"end-datetime"`
}

type tlsrptPolicy struct {
	Policy  tlsrptPolicyDesc   `json:"policy"`
	Summary tlsrptSummary      `json:"summary"`
	Details []tlsrptDetail     `json:"failure-details,omitempty"`
}

type tlsrptPolicyDesc struct {
	PolicyType   string   `json:"policy-type"`
	PolicyString []string `json:"policy-string,omitempty"`
	PolicyDomain string   `json:"policy-domain"`
}

type tlsrptSummary struct {
	TotalSuccessful int `json:"total-successful-session-count"`
	TotalFailure    int `json:"total-failure-session-count"`
}

type tlsrptDetail struct {
	ResultType           string `json:"result-type"`
	SendingMTAIP         string `json:"sending-mta-ip,omitempty"`
	ReceivingMXHostname  string `json:"receiving-mx-hostname,omitempty"`
	FailedSessionCount   int    `json:"failed-session-count"`
	AdditionalInfo       string `json:"additional-information,omitempty"`
}

func (t *TLSReporter) buildReport(domain string, ev *domainTLSEvents) *tlsrptReport {
	now := time.Now().UTC()
	start := now.Add(-t.interval)

	// Group failures by result type
	failuresByType := make(map[string]*tlsrptDetail)
	for _, f := range ev.Failures {
		key := f.ResultType + "|" + f.ReceivingMX
		d, ok := failuresByType[key]
		if !ok {
			d = &tlsrptDetail{
				ResultType:          f.ResultType,
				ReceivingMXHostname: f.ReceivingMX,
				AdditionalInfo:      f.FailureReason,
			}
			failuresByType[key] = d
		}
		d.FailedSessionCount++
	}

	var details []tlsrptDetail
	for _, d := range failuresByType {
		details = append(details, *d)
	}

	return &tlsrptReport{
		OrganizationName: t.senderDomain,
		DateRange: tlsrptDateRange{
			StartDatetime: start.Format(time.RFC3339),
			EndDatetime:   now.Format(time.RFC3339),
		},
		ContactInfo: "tlsrpt@" + t.senderDomain,
		ReportID:    fmt.Sprintf("%s-%s-%d", t.senderDomain, domain, now.Unix()),
		Policies: []tlsrptPolicy{
			{
				Policy: tlsrptPolicyDesc{
					PolicyType:   "sts",
					PolicyDomain: domain,
				},
				Summary: tlsrptSummary{
					TotalSuccessful: ev.SuccessCount,
					TotalFailure:    ev.FailureCount,
				},
				Details: details,
			},
		},
	}
}

// lookupTLSRPT queries the _smtp._tls.<domain> TXT record for the reporting address.
func lookupTLSRPT(domain string) string {
	records, err := net.LookupTXT("_smtp._tls." + domain)
	if err != nil {
		return ""
	}
	for _, txt := range records {
		if !strings.HasPrefix(txt, "v=TLSRPTv1;") && !strings.HasPrefix(txt, "v=TLSRPTv1 ") {
			continue
		}
		// Parse rua= field
		for _, part := range strings.Split(txt, ";") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "rua=") {
				addr := strings.TrimPrefix(part, "rua=")
				addr = strings.TrimPrefix(addr, "mailto:")
				return addr
			}
		}
	}
	return ""
}
