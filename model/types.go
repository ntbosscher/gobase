package model

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
