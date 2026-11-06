package identityhttp

import "testing"

func TestValidatesIdentityConfiguration(t *testing.T) {
	if !validURL("https://rc.ohrats.party") {
		t.Fatal("HTTPS URL should be valid")
	}
	if validURL("javascript:alert(1)") {
		t.Fatal("non-HTTP URL should be invalid")
	}
	if !validURL("https://rc.ohrats.party/path") {
		t.Fatal("valid_url only rejects control characters and spaces")
	}
	if !validAuthority("rc.ohrats.party") {
		t.Fatal("authority should be valid")
	}
	if validAuthority("rc.ohrats.party/evil") {
		t.Fatal("authority with a path must be invalid")
	}
	if !validSetupToken("012345678901234567890123") {
		t.Fatal("graphic setup token should be valid")
	}
	if validSetupToken("short") {
		t.Fatal("short setup token must be invalid")
	}
}

func TestDecodesQueryValuesAndRejectsExternalRedirects(t *testing.T) {
	if value, ok := decodeQueryComponent("a%2Fb+c"); !ok || value != "a/b c" {
		t.Fatalf("decoded value = %q/%v", value, ok)
	}
	if _, ok := decodeQueryComponent("%zz"); ok {
		t.Fatal("invalid escape should be rejected")
	}
	if !safeNext("/devices?x=1") {
		t.Fatal("relative redirect should be safe")
	}
	if safeNext("//attacker.test") || safeNext("https://attacker.test") {
		t.Fatal("external redirect must be rejected")
	}
}

func TestValidatesNamesAndBrowserLifetimes(t *testing.T) {
	name, err := cleanName("  Fern  ")
	if err != nil || name != "Fern" {
		t.Fatalf("name = %q/%v", name, err)
	}
	if _, err := cleanName("\n"); err == nil {
		t.Fatal("control-only name must be rejected")
	}
	seconds, err := lifetimeSeconds("30d")
	if err != nil || seconds != 2_592_000 {
		t.Fatalf("lifetime = %d/%v", seconds, err)
	}
	if _, err := lifetimeSeconds("never"); err == nil {
		t.Fatal("unknown lifetime must be rejected")
	}
}

func TestEscapesAuthenticationAttributes(t *testing.T) {
	result := escapeHTML(`/devices?x="<&`)
	if result != "/devices?x=&quot;&lt;&amp;" {
		t.Fatalf("escaped value = %q", result)
	}
}
