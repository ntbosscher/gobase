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
