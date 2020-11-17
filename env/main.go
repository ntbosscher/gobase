package env

import (
	"github.com/joho/godotenv"
	"log"
	"os"
	"strconv"
)

var IsTesting bool
var IsUnitTest bool

func init() {

	IsUnitTest = os.Getenv("UNIT_TEST") != ""

	err := godotenv.Load()
	if err != nil {

		if IsUnitTest {
			log.Println("Error loading .env file: ", err.Error())
		} else {
			log.Fatal("Error loading .env file: ", err.Error())
		}
	}

	IsTesting = os.Getenv("TEST") == "true"
}

func fatal(message string) {
	if IsUnitTest {
		log.Println(message)
		return
	}

	log.Fatal(message)
}

func Require(key string) string {
	value := os.Getenv(key)
	if value == "" {
		fatal("missing environment variable " + key)
	}

	return value
}

func RequireInt(key string) int {
	i, err := strconv.Atoi(Require(key))
	if err != nil {
		fatal("invalid environment variable value '" + Require(key) + "' for key " + key)
	}

	return i
}

func RequireBool(key string) bool {
	i, err := strconv.ParseBool(Require(key))
	if err != nil {
		fatal("invalid environment variable value '" + Require(key) + "' for key " + key)
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
		fatal("invalid environment variable value '" + v + "' for key " + key + " should be true,false or undefined")
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
		fatal("invalid environment variable value '" + v + "' for key " + key + " should be a number or undefined")
	}

	return i
}
