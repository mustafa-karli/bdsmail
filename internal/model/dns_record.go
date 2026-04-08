package model

import "time"

// DomainDNSRecord represents a DNS record that should be configured for a domain.
type DomainDNSRecord struct {
	Domain     string
	RecordType string // A, MX, TXT, CNAME
	Name       string // @, mail, _dmarc, etc.
	Value      string
	Priority   string
	CreatedAt  time.Time
}
