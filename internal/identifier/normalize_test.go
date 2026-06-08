package identifier

import "testing"

func TestNormalizeColumnName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{input: "TimeStamp", want: "time_stamp"},
		{input: "FrequencyInvert1(Hz)", want: "frequency_invert_1_hz"},
		{input: "Voltagef1_Invert1(V)", want: "voltagef_1_invert_1_v"},
		{input: "THD_V1(%)", want: "thd_v_1_pct"},
		{input: "2VE201(V)", want: "2_ve_201_v"},
		{input: "CurrentIN_inversor1(A)", want: "current_in_inversor_1_a"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.input, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeColumnName(test.input); got != test.want {
				t.Fatalf("NormalizeColumnName(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestQuoteIfNeeded(t *testing.T) {
	t.Parallel()

	if got := QuoteIfNeeded("ts"); got != "ts" {
		t.Fatalf("QuoteIfNeeded(ts) = %q", got)
	}
	if got := QuoteIfNeeded("2_ve_201_v"); got != "\"2_ve_201_v\"" {
		t.Fatalf("QuoteIfNeeded(2_ve_201_v) = %q", got)
	}
}
