package currency

import "testing"

func TestNormalizeSupportedCodes(t *testing.T) {
	if got, ok := Normalize(" usd "); !ok || got != "USD" {
		t.Fatalf("Normalize USD = (%q, %v), want (USD, true)", got, ok)
	}
	if _, ok := Normalize("not-a-currency"); ok {
		t.Fatal("unsupported currency was accepted")
	}
	if _, ok := Normalize(LegacyIRT); !ok {
		t.Fatal("legacy IRT must remain accepted for existing installations")
	}
}

func TestFractionDigits(t *testing.T) {
	cases := map[string]int{"USD": 2, "JPY": 0, "KWD": 3, "TND": 3, "IRT": 2}
	for code, want := range cases {
		if got := FractionDigits(code); got != want {
			t.Errorf("FractionDigits(%s) = %d, want %d", code, got, want)
		}
	}
}
