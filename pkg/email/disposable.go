package email

import (
	_ "embed"
	"net"
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

// IsDisposableEmail checks if the domain of the provided email is in the blocklist.
func IsDisposableEmail(email string) bool {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}
	domain_parts := strings.Split(parts[1], ".")
	for i := 0; i < len(domain_parts)-1; i++ {
		if _, ok := disposableList[strings.Join(domain_parts[i:], ".")]; ok {
			return true
		}
	}
	return false
}

// HasValidMXRecord checks if the DNS records of the domain and verify that it actually has an active MX server.
func HasValidMXRecord(email string) bool {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}

	domain := strings.ToLower(parts[1])

	mxRecords, err := net.LookupMX(domain)
	if err != nil || len(mxRecords) == 0 {
		return false
	}

	return true
}
