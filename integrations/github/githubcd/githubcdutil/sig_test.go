package githubcdutil

import "testing"

func TestT(t *testing.T) {
	str, err := calcSignature("QyzKtDRXrEH3", []byte{})
	if err != nil {
		t.Error(err)
	}

	t.Error(str)
}
