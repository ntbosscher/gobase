package currency

import (
	"encoding/json"
	"testing"
)

func TestParse(t *testing.T) {
	c, err := Parse("2.39")
	if err != nil {
		t.Fatal(err)
	}

	if c != 239 {
		t.Fatal("invalid 2.39")
	}

	c, err = Parse("2.391")
	if err != nil {
		t.Fatal(err)
	}

	if c != 239 {
		t.Fatal("invalid 2.391")
	}

	c, err = Parse("2")
	if err != nil {
		t.Fatal(err)
	}

	if c != 200 {
		t.Fatal("invalid 2")
	}

	c, err = Parse("2.1")
	if err != nil {
		t.Fatal(err)
	}

	if c != 210 {
		t.Fatal("invalid 2.1")
	}
}

func TestParseNegative(t *testing.T) {
	// The sign must apply to the whole amount, not just the dollars.
	cases := map[string]Cents{
		"-2.50":  -250,
		"-0.50":  -50, // regression: previously produced +50 (sign lost)
		"-2":     -200,
		"-0.05":  -5,
		"-1.10":  -110,
		"+2.50":  250,
		"$-2.50": -250,
	}

	for in, want := range cases {
		got, err := Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q) errored: %v", in, err)
		}
		if got != want {
			t.Fatalf("Parse(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestParseRejectsMalformed(t *testing.T) {
	// Embedded signs, stray characters, and empty parts must be rejected
	// rather than silently mis-parsed.
	for _, in := range []string{"2.-5", "2.+5", "2.5x", "abc", "1.2.3", ".", "-.5", "5."} {
		if got, err := Parse(in); err == nil {
			t.Fatalf("Parse(%q) = %d, want an error", in, got)
		}
	}
}

func TestParseOutOfRange(t *testing.T) {
	// A value that would overflow Cents must error, not wrap to a bogus number.
	if got, err := Parse("99999999999999999999"); err == nil {
		t.Fatalf("Parse(huge) = %d, want out-of-range error", got)
	}
}

func TestCents_UnmarshalJSON(t *testing.T) {
	var c CentsWithJsonEncoding
	err := json.Unmarshal([]byte("5.29"), &c)
	if err != nil {
		t.Fatal(err)
	}

	if c != 529 {
		t.Fatal("incorrect 5.29")
	}
}

func TestCents_MarshalJSON(t *testing.T) {
	bytes, _ := json.Marshal(CentsWithJsonEncoding(529))
	if string(bytes) != "5.29" {
		t.Fatal("incorrect 5.29")
	}

	bytes, _ = json.Marshal(CentsWithJsonEncoding(520))
	if string(bytes) != "5.20" {
		t.Fatal("incorrect 5.20")
	}

	bytes, _ = json.Marshal(CentsWithJsonEncoding(20))
	if string(bytes) != "0.20" {
		t.Fatal("incorrect 0.20")
	}

	bytes, _ = json.Marshal(CentsWithJsonEncoding(2))
	if string(bytes) != "0.02" {
		t.Fatal("incorrect 0.02")
	}

	bytes, _ = json.Marshal(CentsWithJsonEncoding(0))
	if string(bytes) != "0.00" {
		t.Fatal("incorrect 0.00")
	}
}

func TestCents_String(t *testing.T) {

	test := map[Cents]string{
		10:   "0.10",
		-10:  "-0.10",
		-110: "-1.10",
		110:  "1.10",
	}

	for k, v := range test {
		if k.String() != v {
			t.Error("invalid formatting for ", v, " got: ", k.String())
		}

		t.Log(k.String())
	}

	for k, v := range test {
		if CentsWithJsonEncoding(k).String() != v {
			t.Error("invalid with-json formatting for ", v, " got: ", k.String())
		}

		t.Log(CentsWithJsonEncoding(k).String())
	}
}
