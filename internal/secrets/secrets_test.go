package secrets

import "testing"

func TestParseRefOpScheme(t *testing.T) {
	provider, key, ok := ParseRef("op://Morningweave/Platform API/x-api-key")
	if !ok {
		t.Fatalf("expected ref to parse")
	}
	if provider != "op" {
		t.Fatalf("unexpected provider: %q", provider)
	}
	if key != "op://Morningweave/Platform API/x-api-key" {
		t.Fatalf("unexpected key: %q", key)
	}
}

func TestParseRefOpPrefix(t *testing.T) {
	provider, key, ok := ParseRef("op:op://Vault/Item/field")
	if !ok {
		t.Fatalf("expected ref to parse")
	}
	if provider != "op" {
		t.Fatalf("unexpected provider: %q", provider)
	}
	if key != "op://Vault/Item/field" {
		t.Fatalf("unexpected key: %q", key)
	}
}

func TestParseRefOpSchemeCaseInsensitive(t *testing.T) {
	provider, key, ok := ParseRef("OP://Vault/Item/field")
	if !ok {
		t.Fatalf("expected ref to parse")
	}
	if provider != "op" {
		t.Fatalf("unexpected provider: %q", provider)
	}
	if key != "op://Vault/Item/field" {
		t.Fatalf("unexpected key: %q", key)
	}
}

func TestParseRefOpProviderWithoutScheme(t *testing.T) {
	provider, key, ok := ParseRef("op:Vault/Item/field")
	if !ok {
		t.Fatalf("expected ref to parse")
	}
	if provider != "op" {
		t.Fatalf("unexpected provider: %q", provider)
	}
	if key != "op://Vault/Item/field" {
		t.Fatalf("unexpected key: %q", key)
	}
}
