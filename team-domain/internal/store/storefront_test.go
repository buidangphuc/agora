package store

import "testing"

type fakeRow struct{ cfg []byte }

func (f fakeRow) Scan(dest ...any) error {
	*dest[0].(*string) = "seller_a"
	*dest[1].(*string) = "shop-a"
	*dest[2].(*[]byte) = f.cfg
	return nil
}

// The Postgres repo stores display_name inside the JSONB config; encode and
// decode must round-trip it (and tolerate legacy rows without the key).
func TestStorefrontConfigRoundTripsDisplayName(t *testing.T) {
	raw, err := encodeConfig(Storefront{DisplayName: "Tiem Hoa Nho", Tagline: "t"})
	if err != nil {
		t.Fatal(err)
	}
	var got Storefront
	if err := scanStorefront(fakeRow{cfg: raw}, &got); err != nil {
		t.Fatal(err)
	}
	if got.DisplayName != "Tiem Hoa Nho" || got.Tagline != "t" {
		t.Errorf("round trip mismatch: %+v", got)
	}
	var legacy Storefront
	if err := scanStorefront(fakeRow{cfg: []byte(`{"tagline":"old"}`)}, &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.DisplayName != "" {
		t.Errorf("legacy row should have empty name, got %q", legacy.DisplayName)
	}
}
