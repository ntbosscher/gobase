package model

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

type NullByteArray struct {
	Valid bool
	Bytes []byte
}

func (n *NullByteArray) Scan(value interface{}) error {
	if value == nil {
		n.Bytes, n.Valid = nil, false
		return nil
	}

	n.Valid = true
	n.Bytes = value.([]byte)
	return nil
}

type Date time.Time

func (d Date) Value() (driver.Value, error) {
	return time.Time(d), nil
}

func (d *Date) Scan(value interface{}) error {
	switch v := value.(type) {
	case time.Time:
		*d = Date(v)
	default:
		return fmt.Errorf("invalid Scan(%T) for model.Date", v)
	}

	return nil
}

const dateFormat = "2006-01-02"

func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	tm, err := time.Parse(dateFormat, s)
	if err != nil {
		return err
	}

	*d = Date(tm)
	return nil
}

func (d Date) MarshalJSON() ([]byte, error) {
	s := time.Time(d).Format(dateFormat)
	return json.Marshal(s)
}

type NullablePoint struct {
	Valid bool
	Y     float64
	X     float64
}

func (p NullablePoint) MarshalJSON() ([]byte, error) {
	if !p.Valid {
		return []byte(`null`), nil
	}

	return json.Marshal(map[string]interface{}{
		"x": p.X,
		"y": p.Y,
	})
}

func (p *NullablePoint) UnmarshalJSON(b []byte) error {
	*p = NullablePoint{}

	if string(b) == "null" {
		return nil
	}

	v := struct {
		X float64
		Y float64
	}{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}

	p.Valid = true
	p.Y = v.Y
	p.X = v.X

	return nil
}

func (p *NullablePoint) Scan(value interface{}) error {
	*p = NullablePoint{}

	if value == nil {
		return nil
	}

	b := value.([]byte)
	b = bytes.Trim(b, "()")
	parts := bytes.Split(b, []byte(","))

	var err error
	p.X, err = strconv.ParseFloat(string(parts[0]), 64)
	if err != nil {
		return err
	}

	p.Y, err = strconv.ParseFloat(string(parts[1]), 64)
	if err != nil {
		return err
	}

	p.Valid = true
	return nil
}

func (p NullablePoint) Value() (driver.Value, error) {
	if !p.Valid {
		return nil, nil
	}
	return fmt.Sprint("(", p.X, ",", p.Y, ")"), nil
}
