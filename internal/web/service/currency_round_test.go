package service

import "testing"

func TestRoundMoneyUsesCurrencyMinorUnits(t *testing.T) {
	cases := []struct {
		code  string
		input float64
		want  float64
	}{
		{code: "USD", input: 1.236, want: 1.24},
		{code: "JPY", input: 1.6, want: 2},
		{code: "KWD", input: 1.2346, want: 1.235},
	}
	for _, tc := range cases {
		if got := RoundMoney(tc.input, tc.code); got != tc.want {
			t.Errorf("RoundMoney(%v, %q) = %v, want %v", tc.input, tc.code, got, tc.want)
		}
	}
}
