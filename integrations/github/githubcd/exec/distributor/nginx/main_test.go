package nginx

import (
	"testing"
)

func TestParse(t *testing.T) {
	input := `
# test
upstream backend {
	# round robin
	server 127.0.0.1:8080;
	
	server localhost:3322 down;
	server localhost:3321 stuff;
}

#suffix`
	info, servers, err := ParseBackends([]byte(input))

	if err != nil {
		t.Fatal(err)
	}

	t.Log(info.String())

	if input != info.String() {
		t.Error("encoding changed")
	}

	if servers[0].Name != "127.0.0.1:8080" {
		t.Error("incorrect servers")
	}

	servers[2].Status = "down"
	t.Log(info.String())

	expected := `
# test
upstream backend {
	# round robin
	server 127.0.0.1:8080;
	
	server localhost:3322 down;
	server localhost:3321 stuff down;
}

#suffix`

	if expected != info.String() {
		t.Error("update didn't work changed")
	}
}
