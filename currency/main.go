package currency

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type Cents int

func (u Cents) String() string {
	sign := ""
	value := u

	if u < 0 {
		sign = "-"
		value = -u
	}

	return fmt.Sprintf("%s%d.%02d", sign, value/100, value%100)
}

type NullCents struct {
	Valid bool
	Cents
}

// Scan implements the Scanner interface.
func (n *NullCents) Scan(value interface{}) error {
	i := sql.NullInt64{}
	if err := i.Scan(value); err != nil {
		return err
	}

	n.Valid = i.Valid
	n.Cents = Cents(i.Int64)
	return nil
}

// Value implements the driver Valuer interface.
func (n NullCents) Value() (driver.Value, error) {
	if !n.Valid {
		return nil, nil
	}

	return int64(n.Cents), nil
}

func (n NullCents) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return []byte("null"), nil
	}

	return json.Marshal(n.Cents)
}

func (n *NullCents) UnmarshalJSON(data []byte) error {

	if string(data) == "null" {
		*n = NullCents{}
		return nil
	}

	*n = NullCents{}
	if err := json.Unmarshal(data, &n.Cents); err != nil {
		return err
	}

	n.Valid = true
	return nil
}

// CentsWithJsonEncoding formats json values in ##.## format
// rather than #### cents format
type CentsWithJsonEncoding Cents

func (u CentsWithJsonEncoding) String() string {
	sign := ""
	value := u

	if u < 0 {
		sign = "-"
		value = -u
	}

	return fmt.Sprintf("%s%d.%02d", sign, value/100, value%100)
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

		if len(parts[1]) == 1 {
			parts[1] = parts[1] + "0"
		}

		cents, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, fmt.Errorf("invalid currency format for string '%s'", src)
		}

		result += Cents(cents)
	}

	return result, nil
}
