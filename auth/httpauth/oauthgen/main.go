package main

import (
	"crypto/rand"
	"log"
)

func main() {
	authKeyLen := 64
	buf := make([]byte, authKeyLen)

	count, err := rand.Read(buf)
	if err != nil {
		log.Fatal(err)
	}

	if count != authKeyLen {
		log.Fatal("failed to fill the buffer")
	}

	// convert buf to ascii characters (! through ~)
	for i := 0; i < len(buf); i++ {
		buf[i] = buf[i]%(126-33) + 33
	}

	log.Println("result: ", string(buf))
}
