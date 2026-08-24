package email

import (
	_ "embed"
	"net"
	"net/mail"
	"strings"
)

//go:embed disposable_email_blocklist.conf
var blocklistData string

var disposableList = make(map[string]struct{}, 3500)

func init() {
	if blocklistData == "" {
		panic("disposable_email_blocklist.conf is missing or empty")
	}

	for line := range strings.SplitSeq(blocklistData, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			disposableList[line] = struct{}{}
		}
	}
}

// IsDisposableDomain checks if the domain is in the blocklist.
func IsDisposableDomain(domain string) bool {
	domainParts := strings.Split(domain, ".")
	for i := 0; i < len(domainParts)-1; i++ {
		if _, ok := disposableList[strings.Join(domainParts[i:], ".")]; ok {
			return true
		}
	}
	return false
}

// HasValidMXRecord checks for the MX record and verify that domain has an active MX server.
func HasValidMXRecord(domain string) bool {
	mxRecords, err := net.LookupMX(domain)
	if err != nil || len(mxRecords) == 0 {
		return false
	}

	return true
}

// HasSPFRecord checks the existence of an SPF record for the domain
func HasSPFRecord(domain string) bool {
	txtRecords, err := net.LookupTXT(domain)
	if err != nil {
		return false
	}

	for _, record := range txtRecords {
		if strings.HasPrefix(strings.ToLower(record), "v=spf1") {
			return true
		}
	}
	return false
}

// HasDMARCRecord checks the existence of a DMARC record
func HasDMARCRecord(domain string) bool {
	txtRecords, err := net.LookupTXT("_dmarc." + domain)
	if err != nil {
		return false
	}
	for _, record := range txtRecords {
		if strings.HasPrefix(strings.ToLower(record), "v=dmarc1") {
			return true
		}
	}
	return false
}

func ValidateAndParseEmail(email string) float64 {
	trustScore := 1.0

	_, err := mail.ParseAddress(email)
	if err != nil {
		return 0
	}

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return 0
	}

	domain := strings.ToLower(parts[1])

	// 1. Check if the domain is in the block list (instant reject)
	// 2. Check if the domain has a valid MX record (instant reject)
	if IsDisposableDomain(domain) || !HasValidMXRecord(domain) {
		return 0
	}

	// TODO: 3. Check if the domain comes from a trusted email provider
	// TODO: 4. Check if the domain has an SPF record that authorizes IPs
	//  belonging to well-known non-mail infrastructure (public DNS resolvers, cloud provider default IPs etc.)
	//  and authorizes a real, known mail-sending IP range (Google, Microsoft etc.)

	// 5. Check if the domain has SPF+DMARC records existing together
	if !HasSPFRecord(email) && !HasDMARCRecord(email) {
		trustScore -= 0.4
	}

	// TODO: 6. Check if the domain has an A record
	// TODO: 7. Check if the domain has a nameserver that is a known bulk/generic DNS host
	// TODO: 8. Check the domain RDAP registration date (if younger than 30 days - red flag)

	if trustScore < 0 {
		return 0
	}

	return trustScore
}
