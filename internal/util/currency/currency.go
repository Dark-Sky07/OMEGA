// Package currency provides the currency codes supported by checkout and the
// minor-unit precision used when calculating and displaying monetary amounts.
package currency

import "strings"

// LegacyIRT is retained for installations that used the former non-ISO IRT
// label. New configurations should use IRR (Iranian rial) instead. No amount
// conversion is performed when changing this display denomination.
const LegacyIRT = "IRT"

// fractionDigits follows ISO 4217 minor units for the currencies exposed by
// the settings UI. Unlisted currencies are rejected instead of guessed.
var fractionDigits = map[string]int{
	"USD": 2, "EUR": 2, "GBP": 2, "JPY": 0, "CNY": 2, "INR": 2,
	"CAD": 2, "AUD": 2, "NZD": 2, "CHF": 2, "SEK": 2, "NOK": 2,
	"DKK": 2, "PLN": 2, "CZK": 2, "HUF": 2, "RON": 2, "TRY": 2,
	"RUB": 2, "UAH": 2, "IRR": 2, "AED": 2, "SAR": 2, "QAR": 2,
	"KWD": 3, "BHD": 3, "OMR": 3, "JOD": 3, "ILS": 2, "BRL": 2,
	"MXN": 2, "ARS": 2, "ZAR": 2, "SGD": 2, "HKD": 2, "TWD": 2,
	"KRW": 0, "IDR": 2, "VND": 0, "THB": 2, "PHP": 2, "MYR": 2,
	"PKR": 2, "EGP": 2, "NGN": 2, "KES": 2, "MAD": 2, "COP": 2,
	"CLP": 0, "TND": 3, "LYD": 3, LegacyIRT: 2,
}

// Normalize returns the uppercase supported code and false for unsupported
// values. IRT remains accepted solely for backwards compatibility.
func Normalize(code string) (string, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	_, ok := fractionDigits[code]
	return code, ok
}

// FractionDigits returns the configured currency's minor-unit precision.
// Unsupported codes use two digits as a defensive fallback; callers that
// accept user input should validate with Normalize first.
func FractionDigits(code string) int {
	if digits, ok := fractionDigits[strings.ToUpper(strings.TrimSpace(code))]; ok {
		return digits
	}
	return 2
}

// Codes returns a stable list of supported codes, excluding the legacy label
// from normal choices. It is kept here to make backend validation explicit.
func Codes() []string {
	return []string{
		"USD", "EUR", "GBP", "JPY", "CNY", "INR", "CAD", "AUD", "NZD", "CHF",
		"SEK", "NOK", "DKK", "PLN", "CZK", "HUF", "RON", "TRY", "RUB", "UAH",
		"IRR", "AED", "SAR", "QAR", "KWD", "BHD", "OMR", "JOD", "ILS", "BRL",
		"MXN", "ARS", "ZAR", "SGD", "HKD", "TWD", "KRW", "IDR", "VND", "THB",
		"PHP", "MYR", "PKR", "EGP", "NGN", "KES", "MAD", "COP", "CLP", "TND", "LYD",
	}
}
