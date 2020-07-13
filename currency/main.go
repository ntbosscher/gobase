package currency

import (
	"fmt"
	"strconv"
	"strings"
)

type Cents int

func (u Cents) String() string {
	return fmt.Sprintf("%d.%02d", u/100, u%100)
}

type CentsWithJsonEncoding Cents

func (u CentsWithJsonEncoding) String() string {
	return fmt.Sprintf("%d.%02d", u/100, u%100)
}

func (u CentsWithJsonEncoding) MarshalJSON() ([]byte, error) {
	return []byte(u.String()), nil
}

func (u *CentsWithJsonEncoding) UnmarshalJSON(data []byte) error {
	c, err := Parse(string(data))
	if err != nil {
		return err
	}

	*u = CentsWithJsonEncoding(c)
	return nil
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
		if len(parts[1]) > 2 {
			// warning, truncating trailing cents
			parts[1] = parts[1][0:2]
		}

		cents, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, fmt.Errorf("invalid currency format for string '%s'", src)
		}

		result += Cents(cents)
	}

	return result, nil
}
