package random

import (
	"crypto/rand"
	"errors"
	"math/big"
)

func randomString(length int, charset string) (string, error) {
	charsetSize := len(charset)
	if charsetSize == 0 {
		return "", errors.New("random: charset must not be empty")
	}
	if charsetSize > 256 {
		return "", errors.New("random: charset must be <= 256 chars for byte-based sampling")
	}

	// Largest multiple of charsetSize that fits in a byte. Bytes at or above
	// this threshold are rejected so that `% charsetSize` doesn't over-represent
	// the low end of the charset (modulo bias). When charsetSize divides 256
	// evenly, maxUnbiased == 256 and nothing is ever rejected.
	maxUnbiased := 256 - (256 % charsetSize)

	b := make([]byte, length)
	buf := make([]byte, length)
	filled := 0

	for filled < length {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}

		for _, v := range buf {
			if int(v) >= maxUnbiased {
				continue // reject to avoid modulo bias
			}

			b[filled] = charset[int(v)%charsetSize]
			filled++
			if filled == length {
				break
			}
		}
	}

	return string(b), nil
}

func GetNumericChars(length int) (string, error) {
	return randomString(length, "0123456789")
}

func GetHexChars(length int) (string, error) {
	return randomString(length, "0123456789abcdef")
}

func GetLowerAlphaNumericChars(length int) (string, error) {
	return randomString(length, "0123456789abcdefghijklmnopqrstuvwxyz")
}

func GetAlphaNumericChars(length int) (string, error) {
	return randomString(length, "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
}

// Int returns a cryptographically random number in [min, max).
// It returns an error if max <= min (rather than panicking).
func Int(min int, max int) (int, error) {

	if max <= min {
		return 0, errors.New("random: max must be greater than min")
	}

	diff := max - min
	value, err := rand.Int(rand.Reader, big.NewInt(int64(diff)))
	if err != nil {
		return 0, err
	}

	return min + int(value.Int64()), nil
}
