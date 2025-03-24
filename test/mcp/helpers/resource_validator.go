package helpers

import (
	"strings"

	"github.com/cowgnition/cowgnition/pkg/util/stringutil"
	urlutil "github.com/cowgnition/cowgnition/pkg/util/url"
)

// ExtractAuthInfoFromContent attempts to extract auth URL and frob from content.
func ExtractAuthInfoFromContent(content string) (string, string) {
	// Look for URL in content.
	urlIdx := strings.Index(content, "https://www.rememberthemilk.com/services/auth/")
	if urlIdx == -1 {
		return "", ""
	}

	// Extract URL.
	endURLIdx := urlutil.FindURLEndIndex(content, urlIdx)
	authURL := content[urlIdx:endURLIdx]

	// Try to extract frob, first from URL then from content text.
	frob, err := urlutil.ExtractQueryParam(authURL, "frob")
	if err != nil || frob == "" {
		// If frob not found in URL, look in content text.
		patterns := []string{
			"frob ",
			"frob: ",
			"Frob: ",
			"frob=",
			"\"frob\": \"",
		}
		frob = stringutil.ExtractFromContent(content, patterns)
	}

	return authURL, frob
}
