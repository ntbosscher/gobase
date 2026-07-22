package main

import (
	"crypto/rand"
	"log"
	"os"
)

func main() {
	rsaKeyLen := 2048
	buf := make([]byte, rsaKeyLen)

	i, err := rand.Read(buf)
	if err != nil {
		log.Fatal(err)
	}

	if i != rsaKeyLen {
		log.Fatal("failed to fill the buffer")
	}

	file := "./.jwtkey"

	if err := os.WriteFile(file, buf, 0o600); err != nil {
		log.Fatal(err)
	}

	// WriteFile only applies the mode when creating the file; chmod ensures the
	// key is tightened even if it already existed with looser permissions.
	if err := os.Chmod(file, 0o600); err != nil {
		log.Fatal(err)
	}

	log.Println("wrote cryptographically random bytes to " + file)
}
