package random

import (
	"crypto/rand"
	"math/big"
)

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

// Int returns a cryptographically random number between [min, max)
func Int(min int, max int) (int, error) {

	diff := max - min
	value, err := rand.Int(rand.Reader, big.NewInt(int64(diff)))
	if err != nil {
		return 0, err
	}

	return min + int(value.Int64()), nil
}
