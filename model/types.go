package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
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
