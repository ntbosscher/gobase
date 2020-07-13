package currency

import (
	"encoding/json"
	"testing"
)

func TestCents_MarshalJSON(t *testing.T) {
	bytes, _ := json.Marshal(Cents(529))
	if string(bytes) != "5.29" {
		t.Fatal("incorrect 5.29")
	}

	bytes, _ = json.Marshal(Cents(520))
	if string(bytes) != "5.20" {
		t.Fatal("incorrect 5.20")
	}

	bytes, _ = json.Marshal(Cents(20))
	if string(bytes) != "0.20" {
		t.Fatal("incorrect 0.20")
	}

	bytes, _ = json.Marshal(Cents(2))
	if string(bytes) != "0.02" {
		t.Fatal("incorrect 0.02")
	}

	bytes, _ = json.Marshal(Cents(0))
	if string(bytes) != "0.00" {
		t.Fatal("incorrect 0.00")
	}
}
