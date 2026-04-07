package dns

import (
	"fmt"
	"net"
	"strings"
)

// VerifyMX checks that the domain has an MX record pointing to the expected hostname.
func VerifyMX(domain, expectedHost string) (bool, error) {
	mxs, err := net.LookupMX(domain)
	if err != nil {
		return false, fmt.Errorf("MX lookup failed for %s: %w", domain, err)
	}
	expected := strings.ToLower(strings.TrimSuffix(expectedHost, "."))
	for _, mx := range mxs {
		host := strings.ToLower(strings.TrimSuffix(mx.Host, "."))
		if host == expected {
			return true, nil
		}
	}
	return false, nil
}

// VerifyDomainOwnership verifies that the domain's MX record points to the expected mail hostname.
// This is the primary ownership proof — only the domain owner can set MX records.
func VerifyDomainOwnership(domain, mailHostname string) error {
	found, err := VerifyMX(domain, mailHostname)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("MX record for %s does not point to %s", domain, mailHostname)
	}
	return nil
}
