package main

import (
	"crypto/rand"
	"io/ioutil"
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

	if err := ioutil.WriteFile(file, buf, os.ModePerm); err != nil {
		log.Fatal(err)
	}

	log.Println("wrote cryptographically random bytes to " + file)
}
