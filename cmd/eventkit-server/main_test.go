package main

import "testing"

func TestValidatePublicURL(t *testing.T) {
	ok := []string{
		"https://apple-gpt.dbee.me",
		"https://host.example.com",
		"http://127.0.0.1:8765",
		"https://host", // bare host, no TLD — still a valid origin
		"https://host/",
	}
	for _, s := range ok {
		if err := validatePublicURL(s); err != nil {
			t.Errorf("validatePublicURL(%q) = %v, want nil", s, err)
		}
	}

	bad := []string{
		"apple-gpt.dbee.me", // no scheme
		"ftp://host",        // wrong scheme
		"https://",          // no host
		"https://host/v1",   // has a path
		"https://host?x=1",  // has a query
		"https://host#frag", // has a fragment
	}
	for _, s := range bad {
		if err := validatePublicURL(s); err == nil {
			t.Errorf("validatePublicURL(%q) = nil, want error", s)
		}
	}
}

func TestParseConfig_PublicURLFlagAndValidation(t *testing.T) {
	cfg, err := parseConfig([]string{"--public-url", "https://apple-gpt.dbee.me"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.publicURL != "https://apple-gpt.dbee.me" {
		t.Errorf("publicURL = %q, want https://apple-gpt.dbee.me", cfg.publicURL)
	}

	if _, err := parseConfig([]string{"--public-url", "not-a-url"}); err == nil {
		t.Errorf("expected parseConfig to reject an invalid --public-url")
	}
}
