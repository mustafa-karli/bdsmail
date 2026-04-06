package awsutil

import (
	"context"
	"fmt"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ses"
)

// SESClient wraps AWS SES operations for domain verification.
type SESClient struct {
	client *ses.Client
	region string
}

// DomainVerification holds the result of initiating SES domain verification.
type DomainVerification struct {
	Domain     string
	DKIMTokens []string // 3 CNAME tokens for SES DKIM
	Status     string   // "Pending", "Success", "Failed"
}

// NewSESClient creates an SES client for the given region.
func NewSESClient(region string) (*SESClient, error) {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	return &SESClient{client: ses.NewFromConfig(cfg), region: region}, nil
}

// VerifyDomain initiates domain verification and DKIM setup in SES.
// Returns DKIM tokens that need to be added as CNAME records.
func (s *SESClient) VerifyDomain(ctx context.Context, domain string) (*DomainVerification, error) {
	// Verify the domain identity
	_, err := s.client.VerifyDomainIdentity(ctx, &ses.VerifyDomainIdentityInput{
		Domain: &domain,
	})
	if err != nil {
		return nil, fmt.Errorf("SES VerifyDomainIdentity failed: %w", err)
	}

	// Enable DKIM for the domain
	dkimOutput, err := s.client.VerifyDomainDkim(ctx, &ses.VerifyDomainDkimInput{
		Domain: &domain,
	})
	if err != nil {
		return nil, fmt.Errorf("SES VerifyDomainDkim failed: %w", err)
	}

	return &DomainVerification{
		Domain:     domain,
		DKIMTokens: dkimOutput.DkimTokens,
		Status:     "Pending",
	}, nil
}

// CheckVerificationStatus checks if a domain has been verified in SES.
func (s *SESClient) CheckVerificationStatus(ctx context.Context, domain string) (string, error) {
	output, err := s.client.GetIdentityVerificationAttributes(ctx, &ses.GetIdentityVerificationAttributesInput{
		Identities: []string{domain},
	})
	if err != nil {
		return "", fmt.Errorf("SES GetIdentityVerificationAttributes failed: %w", err)
	}

	attrs, ok := output.VerificationAttributes[domain]
	if !ok {
		return "NotStarted", nil
	}
	return string(attrs.VerificationStatus), nil
}

// CheckDKIMStatus checks if DKIM has been verified for a domain.
func (s *SESClient) CheckDKIMStatus(ctx context.Context, domain string) (string, error) {
	output, err := s.client.GetIdentityDkimAttributes(ctx, &ses.GetIdentityDkimAttributesInput{
		Identities: []string{domain},
	})
	if err != nil {
		return "", fmt.Errorf("SES GetIdentityDkimAttributes failed: %w", err)
	}

	attrs, ok := output.DkimAttributes[domain]
	if !ok {
		return "NotStarted", nil
	}
	return string(attrs.DkimVerificationStatus), nil
}
