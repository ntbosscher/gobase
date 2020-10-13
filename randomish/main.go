// Package randomish has utils for generating random strings.
// It's called random-ish b/c it's not truly random (we're using math/rand with a UnixNano() seed). But it's random
// enough for most things
package randomish

import (
	"math/rand"
	"time"
)

var random = rand.New(rand.NewSource(time.Now().UnixNano()))

func randomString(length int, charset string) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[random.Intn(len(charset))]
	}
	return string(b)
}

func GetAlphaNumericChars(length int) string {
	return randomString(length, "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
}

func Int(min int, max int) int {
	return rand.Intn(max-min) + min
}
