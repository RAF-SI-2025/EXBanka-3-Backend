package interbank

import "testing"

func TestNewRegistryFromJSON_Valid(t *testing.T) {
	reg, err := NewRegistryFromJSON(333, `[
		{"code":111,"baseUrl":"http://a/","outboundKey":"o1","inboundKey":"i1","displayName":"Bank A"},
		{"code":222,"baseUrl":"http://b","outboundKey":"o2","inboundKey":"i2"}
	]`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg.OwnRoutingNumber() != 333 {
		t.Errorf("own routing = %d, want 333", reg.OwnRoutingNumber())
	}
	p := reg.Lookup(111)
	if p == nil || p.BaseURL != "http://a" { // trailing slash trimmed
		t.Errorf("Lookup(111) = %+v", p)
	}
	if reg.Lookup(999) != nil {
		t.Error("Lookup(999) should be nil")
	}
	if reg.LookupByInboundKey("i2") == nil {
		t.Error("LookupByInboundKey(i2) should resolve")
	}
	if reg.LookupByInboundKey("") != nil || reg.LookupByInboundKey("nope") != nil {
		t.Error("unknown inbound keys must be nil")
	}
	if all := reg.All(); len(all) != 2 {
		t.Errorf("All() len = %d, want 2", len(all))
	}
}

func TestNewRegistryFromJSON_Empty(t *testing.T) {
	reg, err := NewRegistryFromJSON(333, "   ")
	if err != nil || reg == nil {
		t.Fatalf("empty partners JSON should be ok: %v", err)
	}
	if len(reg.All()) != 0 {
		t.Error("expected no partners")
	}
}

func TestNewRegistryFromJSON_Errors(t *testing.T) {
	cases := map[string]string{
		"invalid json":     `{not json`,
		"missing code":     `[{"baseUrl":"http://a","outboundKey":"o","inboundKey":"i"}]`,
		"own collision":    `[{"code":333,"baseUrl":"http://a","outboundKey":"o","inboundKey":"i"}]`,
		"missing baseurl":  `[{"code":111,"outboundKey":"o","inboundKey":"i"}]`,
		"missing keys":     `[{"code":111,"baseUrl":"http://a","outboundKey":"o"}]`,
		"dup code":         `[{"code":111,"baseUrl":"http://a","outboundKey":"o","inboundKey":"i1"},{"code":111,"baseUrl":"http://b","outboundKey":"o2","inboundKey":"i2"}]`,
		"dup inbound key":  `[{"code":111,"baseUrl":"http://a","outboundKey":"o","inboundKey":"i"},{"code":222,"baseUrl":"http://b","outboundKey":"o2","inboundKey":"i"}]`,
	}
	for name, j := range cases {
		if _, err := NewRegistryFromJSON(333, j); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestRoutingNumberFromAccount(t *testing.T) {
	if n, err := RoutingNumberFromAccount("333000123"); err != nil || n != 333 {
		t.Errorf("got %d, %v", n, err)
	}
	if _, err := RoutingNumberFromAccount("ab"); err == nil {
		t.Error("too short: want error")
	}
	if _, err := RoutingNumberFromAccount("abc000"); err == nil {
		t.Error("non-numeric prefix: want error")
	}
}

func TestResolveBankFromAccount(t *testing.T) {
	reg := testRegistry(t, 333, 111, "http://partner")

	code, url, local, err := reg.ResolveBankFromAccount("333000999")
	if err != nil || !local || code != 333 || url != "" {
		t.Errorf("own account: got code=%d url=%q local=%v err=%v", code, url, local, err)
	}

	code, url, local, err = reg.ResolveBankFromAccount("111000999")
	if err != nil || local || code != 111 || url != "http://partner" {
		t.Errorf("partner account: got code=%d url=%q local=%v err=%v", code, url, local, err)
	}

	if _, _, _, err := reg.ResolveBankFromAccount("999000999"); err == nil {
		t.Error("unregistered routing: want error")
	}
	if _, _, _, err := reg.ResolveBankFromAccount("xx"); err == nil {
		t.Error("bad account: want error")
	}
}
