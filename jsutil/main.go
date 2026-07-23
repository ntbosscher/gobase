package jsutil

import (
	"bytes"
	"encoding/json"

	jsoniter "github.com/json-iterator/go"
	"github.com/ntbosscher/gobase/er"
	_ "github.com/ntbosscher/gobase/res" // registers the global camelCase naming strategy so casing matches res output
)

// encoder is pinned locally. EscapeHTML escapes <, >, & so JSON
// dropped into a <script> block can't break out (e.g. via "</script>").
// This matches res's jsoniter.ConfigDefault; key casing still comes from the
// global naming-strategy extension registered by res's init.
var encoder = jsoniter.Config{EscapeHTML: true}.Froze()

// U+2028 (line separator) / U+2029 (paragraph separator): raw UTF-8 encoding
// vs. the "\\u2028"/"\\u2029" escape (6 ASCII bytes) we replace it with.
var (
	lineSep    = []byte("\u2028")
	lineSepEsc = []byte("\\u2028")
	paraSep    = []byte("\u2029")
	paraSepEsc = []byte("\\u2029")
)

// Encode marshals input to JSON that is safe to embed directly inside an HTML
// <script> block.
func Encode(input any) json.RawMessage {
	content, err := encoder.Marshal(input)
	er.Check(err)

	// U+2028/U+2029 are valid in JSON but are line terminators in JavaScript,
	// so unescaped they break the embedded script. EscapeHTML doesn't touch
	// them; escape them here. UTF-8 is self-synchronizing, so these byte
	// sequences only ever appear as the real code points inside string values,
	// making the replacement safe.
	content = bytes.ReplaceAll(content, lineSep, lineSepEsc)
	content = bytes.ReplaceAll(content, paraSep, paraSepEsc)

	return content
}

func EncodeString(input any) string {
	return string(Encode(input))
}
