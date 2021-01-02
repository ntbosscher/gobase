package random

import "testing"

func TestGetAlphaNumericChars(t *testing.T) {
	result, err := GetAlphaNumericChars(32)
	if err != nil {
		t.Fatal(err)
	}

	t.Log(result)
}

func BenchmarkGetAlphaNumericChars(t *testing.B) {
	for i := 0; i < t.N; i++ {
		_, err := GetAlphaNumericChars(32)
		if err != nil {
			t.Fatal(err)
		}
	}
}
