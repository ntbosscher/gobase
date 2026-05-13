package jsutil

import (
	"encoding/json"

	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/res"
)

func Encode(input any) json.RawMessage {
	// ensure casing matches
	content, err := res.GetJSONInstance().Marshal(input)
	er.Check(err)

	return content
}

func EncodeString(input any) string {
	return string(Encode(input))
}
