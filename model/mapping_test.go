package model

import (
	"testing"
)

func TestSnakeCaseStructNameMapping(t *testing.T) {

	tests := map[string]string{
		"ID":               "id",
		"UserID":           "user_id",
		"User":             "user",
		"UserWorkshop":     "user_workshop",
		"Transponder407":   "transponder_407",
		"Transponder4":     "transponder_4",
		"Transponder4Free": "transponder_4_free",
		"NLunches":         "nlunches",
	}

	for input, expect := range tests {
		got := SnakeCaseStructNameMapping(input)
		if got != expect {
			t.Errorf("incorrect result for '%s' -> '%s', got '%s'", input, expect, got)
		}
	}
}
