package gen

import (
	"crypto/rand"
	"io/ioutil"
	"os"
	"testing"
)

func TestJWTGen(t *testing.T) {

	rsaKeyLen := 2048
	buf := make([]byte, rsaKeyLen)

	i, err := rand.Read(buf)
	if err != nil {
		t.Fatal(err)
	}

	if i != rsaKeyLen {
		t.Fatal("failed to fill the buffer")
	}

	if err := ioutil.WriteFile("./jwtkey", buf, os.ModePerm); err != nil {
		t.Fatal(err)
	}
}
