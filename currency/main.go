package currency

import (
	"fmt"
	"strconv"
	"strings"
)

type Cents int

func (u Cents) MarshalJSON() ([]byte, error) {
	str := fmt.Sprintf("%d.%02d", u/100, u%100)
	return []byte(str), nil
}

func Parse(src string) (Cents, error) {
	sanitized := src
	sanitized = strings.Trim(sanitized, "$ ")
	sanitized = strings.Replace(sanitized, ",", "", -1)

	parts := strings.Split(sanitized, ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("invalid currency format for string '%s'", src)
	}

	dollars, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid currency format for string '%s'", src)
	}

	result := Cents(dollars * 100)

	if len(parts) == 2 {
		cents, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, fmt.Errorf("invalid currency format for string '%s'", src)
		}

		result += Cents(cents)
	}

	return result, nil
}
