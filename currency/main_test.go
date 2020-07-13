package currency

import (
	"encoding/json"
	"testing"
)

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
