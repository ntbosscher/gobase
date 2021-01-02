package random

import "crypto/rand"

func randomString(length int, charset string) (string, error) {
	buf := make([]byte, length)
	_, err := rand.Read(buf)
	if err != nil {
		return "", err
	}

	b := make([]byte, length)
	charsetSize := len(charset)

	for i := range b {
		b[i] = charset[int(buf[i])%charsetSize]
	}

	return string(b), nil
}

func GetAlphaNumericChars(length int) (string, error) {
	return randomString(length, "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
}
