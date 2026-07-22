package currency

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"
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
	invalid := func() (Cents, error) {
		return 0, fmt.Errorf("invalid currency format for string '%s'", src)
	}

	sanitized := strings.Trim(src, "$ ")
	sanitized = strings.Replace(sanitized, ",", "", -1)

	// Extract a single leading sign for the whole amount, then parse the
	// magnitude as non-negative.
	negative := false
	switch {
	case strings.HasPrefix(sanitized, "-"):
		negative = true
		sanitized = sanitized[1:]
	case strings.HasPrefix(sanitized, "+"):
		sanitized = sanitized[1:]
	}

	parts := strings.Split(sanitized, ".")
	if len(parts) > 2 {
		return invalid()
	}

	// Digits only — rejects an embedded sign (e.g. "2.-5") or any stray
	// characters that Atoi would otherwise accept.
	if !isAllDigits(parts[0]) {
		return invalid()
	}

	dollars, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return invalid()
	}

	// Bound the dollar magnitude before multiplying so the int64 math below
	// can't overflow (and then silently wrap to a bogus/negative value).
	const maxDollars = (math.MaxInt64 - 99) / 100
	if dollars > maxDollars {
		return 0, fmt.Errorf("currency value out of range for string '%s'", src)
	}

	total := dollars * 100

	if len(parts) == 2 {
		frac := parts[1]
		if len(frac) > 2 {
			// warning, truncating trailing cents beyond 2 decimal places
			frac = frac[0:2]
		}

		if len(frac) == 1 {
			frac = frac + "0"
		}

		if !isAllDigits(frac) {
			return invalid()
		}

		cents, err := strconv.ParseInt(frac, 10, 64)
		if err != nil {
			return invalid()
		}

		total += cents
	}

	if negative {
		total = -total
	}

	// Narrow to the platform-dependent Cents (int) and reject anything that
	// wouldn't fit rather than wrapping around.
	if total > math.MaxInt || total < math.MinInt {
		return 0, fmt.Errorf("currency value out of range for string '%s'", src)
	}

	return Cents(total), nil
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}

	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}

	return true
}
