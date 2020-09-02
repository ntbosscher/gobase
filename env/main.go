package env

import (
	"github.com/joho/godotenv"
	"log"
	"os"
	"strconv"
)

var IsTesting bool

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file: ", err.Error())
	}

	IsTesting = os.Getenv("TEST") == "true"
}

func Require(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatal("missing environment variable " + key)
	}

	return value
}

func RequireInt(key string) int {
	i, err := strconv.Atoi(Require(key))
	if err != nil {
		log.Fatal("invalid environment variable value '" + Require(key) + "' for key " + key)
	}

	return i
}

func RequireBool(key string) bool {
	i, err := strconv.ParseBool(Require(key))
	if err != nil {
		log.Fatal("invalid environment variable value '" + Require(key) + "' for key " + key)
	}

	return i
}

func Optional(key string, defaultValue string) string {
	v := os.Getenv(key)
	if v != "" {
		return v
	}

	return defaultValue
}

func OptionalBool(key string, defaultValue bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}

	i, err := strconv.ParseBool(v)
	if err != nil {
		log.Fatal("invalid environment variable value '" + v + "' for key " + key + " should be true,false or undefined")
	}

	return i
}

func OptionalInt(key string, defaultValue int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}

	i, err := strconv.Atoi(v)
	if err != nil {
		log.Fatal("invalid environment variable value '" + v + "' for key " + key + " should be a number or undefined")
	}

	return i
}
