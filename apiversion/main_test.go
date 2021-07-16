package apiversion

import "testing"

func TestComparison(t *testing.T) {

	trues := map[string]string{
		"1.0":   "1",
		"1.0.0": "1",
		"1":     "1",
		"1.0.1": "1",
		"1.1":   "1",
		"1.2":   "1.1.2",
		"0":     "",
	}

	falses := map[string]string{
		"0.2": "1.1.2",
		"":    "0",
		"1":   "2",
	}

	for k, v := range trues {
		if !NewVer(k).GtOrEq(NewVer(v)) {
			t.Fatal("expected true for '" + k + "' >= '" + v + "'")
		}
	}

	for k, v := range falses {
		if NewVer(k).GtOrEq(NewVer(v)) {
			t.Fatal("expected false for '" + k + "' >= '" + v + "'")
		}
	}
}
