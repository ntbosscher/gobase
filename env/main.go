package env

import (
	"github.com/joho/godotenv"
	"log"
	"os"
)

var IsTesting bool

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file: ", err.Error())
	}

	IsTesting = os.Getenv("TEST") != ""
}

func Require(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatal("missing environment variable " + key)
	}

	return value
}

func Optional(key string) string {
	return os.Getenv(key)
}
