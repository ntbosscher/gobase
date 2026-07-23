package res

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"reflect"
	"strings"
	"time"
	"unicode"

	jsoniter "github.com/json-iterator/go"
	"github.com/json-iterator/go/extra"
	"github.com/ntbosscher/gobase/er"
)

var json = jsoniter.ConfigDefault
var errorLogger *log.Logger
var ignoreErrorLog ErrorLogFilterer

type ErrorLogFilterer func(responseCode int, responseBody interface{}, request *http.Request) bool

func init() {
	extra.SetNamingStrategy(JsonRenameKeysToCamelCase)
	SetErrorResponseLogging(ioutil.Discard)
	IgnoreErrorLogFor(func(responseCode int, responseBody interface{}, request *http.Request) bool {
		return false // log all errors
	})
}

func GetJSONInstance() jsoniter.API {
	return json
}

// SetErrorResponseLogging determines where to pipe http errors
// by default errors are sent to /dev/null
func SetErrorResponseLogging(writer io.Writer) {
	errorLogger = log.New(writer, "http: ", log.Ltime&log.Ldate)
}

func IgnoreErrorLogFor(callback ErrorLogFilterer) {
	ignoreErrorLog = callback
}

func JsonRenameKeysToCamelCase(key string) string {

	if len(key) == 0 {
		return key
	}

	if key == "ID" {
		return "id"
	}

	runes := []rune(key)
	runes[0] = unicode.ToLower(runes[0])

	length := len(runes)

	if length > 2 {
		if string(runes[length-2:]) == "ID" {
			runes[length-2] = 'I'
			runes[length-1] = 'd'
		}
	}

	return string(runes)
}

type Responder interface {
	Respond(w http.ResponseWriter, r *http.Request)
}

type responder struct {
	status int
	data   interface{}
}

func (resp *responder) Respond(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.status)

	if resp.status >= 400 {
		if !ignoreErrorLog(resp.status, resp.data, r) {

			debugStr := ""

			if mapValue, isMap := resp.data.(map[string]interface{}); isMap {
				// write out the key = value per line to make debugging easier
				bld := strings.Builder{}
				bld.WriteString("\n")
				for k, v := range mapValue {
					bld.WriteString("    ")
					bld.WriteString(k)
					bld.WriteString(":  ")
					bld.WriteString(fmt.Sprint(v))
					bld.WriteString("\n")
				}

				debugStr = bld.String()
			} else {
				// Who knows what type of object this is. Let's just json-encode it so the developer
				// can read it.
				js, _ := json.MarshalIndent(resp.data, "", "   ")
				jsStr := string(js)
				if jsStr != `""` {
					jsStr = "\n" + jsStr
				} else {
					jsStr = ""
				}

				debugStr = jsStr
			}

			errorLogger.Printf("request failed: %s %s -> %d%s", r.Method, r.URL, resp.status, debugStr)
		}
	}

	if bodyAllowedForStatus(resp.status) {
		if err := json.NewEncoder(w).Encode(resp.data); err != nil {
			log.Println(err)
		}
	} else if resp.data != nil {
		log.Println("body not allowed for status, but .data was not nil")
	}
}

// bodyAllowedForStatus reports whether a given response status code
// permits a body. See RFC 7230, section 3.3.
func bodyAllowedForStatus(status int) bool {
	switch {
	case status >= 100 && status <= 199:
		return false
	case status == 204:
		return false
	case status == 304:
		return false
	}
	return true
}

func Html(str string) Responder {
	return &freeformResponder{
		respond: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(str))
		},
	}
}

func Func(method func(w http.ResponseWriter)) Responder {
	return &freeformResponder{respond: func(w http.ResponseWriter, r *http.Request) {
		method(w)
	}}
}

func WrapHTTP(server http.Handler) Responder {
	return &freeformResponder{respond: func(w http.ResponseWriter, r *http.Request) {
		server.ServeHTTP(w, r)
	}}
}

// Error returns a client-safe responder for err. Following the same disclosure
// policy as the panic handler, the underlying error is logged server-side while
// the client receives only a generic message — unless er.ReturnErrorMessageToClient
// is set (typically dev mode), in which case the detail is passed through. This
// prevents raw DB/driver/internal error text from leaking to callers.
func Error(err error) Responder {
	if err == nil {
		return AppError(er.GenericErrorMessage)
	}

	er.ErrorLog.Println(err.Error())

	if er.ReturnErrorMessageToClient {
		return AppError(err.Error())
	}

	return AppError(er.GenericErrorMessage)
}

type freeformResponder struct {
	respond func(w http.ResponseWriter, r *http.Request)
}

func (resp *freeformResponder) Respond(w http.ResponseWriter, r *http.Request) {
	resp.respond(w, r)
}

func Download(name string, data io.ReadSeeker) Responder {
	return &freeformResponder{
		respond: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Content-Disposition", contentDisposition("attachment", name))
			http.ServeContent(w, r, name, time.Now(), data)

			if cl, ok := data.(io.Closer); ok {
				_ = cl.Close()
			}
		},
	}
}

// contentDisposition builds an RFC 6266-compliant Content-Disposition header
// value for a user-supplied filename. It emits a quoted, ASCII-only filename
// (control bytes incl. CR/LF, quotes and backslashes removed so the value can
// neither break the header nor inject extra parameters) plus a UTF-8 filename*
// per RFC 5987 so non-ASCII names survive intact in modern clients.
func contentDisposition(disposition, fileName string) string {
	out := disposition + `; filename="` + SanitizeDispositionName(fileName) + `"`

	if hasNonASCII(fileName) {
		out += `; filename*=UTF-8''` + encodeRFC5987(fileName)
	}

	return out
}

// SanitizeDispositionName returns an ASCII, header-safe filename for use inside
// a quoted Content-Disposition filename parameter. It drops control bytes
// (including CR/LF, which would otherwise allow header injection), non-ASCII
// bytes (carried by filename* instead), the double-quote and backslash (which
// are special inside a quoted-string), and commas (a legacy Chrome bug:
// https://answers.nuxeo.com/general/q/d8348e07fe5e441183bae07dfda00e40/Comma-in-file-name-cause-problem-in-Chrome-Browser).
func SanitizeDispositionName(fileName string) string {
	var b strings.Builder

	for _, ru := range fileName {
		switch {
		case ru < 0x20 || ru == 0x7f: // control chars incl. CR/LF
		case ru > 0x7f: // non-ASCII
		case ru == '"' || ru == '\\' || ru == ',':
		default:
			b.WriteRune(ru)
		}
	}

	return b.String()
}

func hasNonASCII(s string) bool {
	for _, ru := range s {
		if ru > 0x7f {
			return true
		}
	}

	return false
}

// encodeRFC5987 percent-encodes s per the RFC 5987 ext-value grammar: every
// octet outside the attr-char set is %XX-encoded (uppercase hex).
func encodeRFC5987(s string) string {
	const upperhex = "0123456789ABCDEF"

	var b strings.Builder

	for i := 0; i < len(s); i++ {
		c := s[i]
		if isAttrChar(c) {
			b.WriteByte(c)
			continue
		}

		b.WriteByte('%')
		b.WriteByte(upperhex[c>>4])
		b.WriteByte(upperhex[c&0x0f])
	}

	return b.String()
}

func isAttrChar(c byte) bool {
	switch {
	case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		return true
	}

	switch c {
	case '!', '#', '$', '&', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	}

	return false
}

func Display(name string, data io.ReadSeeker) Responder {
	return &freeformResponder{
		respond: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Content-Disposition", contentDisposition("inline", name))
			http.ServeContent(w, r, name, time.Now(), data)

			if cl, ok := data.(io.Closer); ok {
				_ = cl.Close()
			}
		},
	}
}

func Ok(data ...interface{}) Responder {

	var dt interface{}

	if len(data) == 0 {
		dt = map[string]interface{}{
			"ok": true,
		}
	} else if len(data) == 1 {
		dt = data[0]
	} else {
		dt = data
	}

	dt = fixNilList(dt)

	return &responder{
		status: http.StatusOK,
		data:   dt,
	}
}

func fixNilList(input interface{}) interface{} {
	typ := reflect.TypeOf(input)

	// interface is untyped nil
	if typ == nil {
		return input
	}

	switch typ.Kind() {
	case reflect.Slice:
		fallthrough
	case reflect.Array:
		if reflect.ValueOf(input).IsNil() {
			return []int{}
		}
	}

	return input
}

func List(list interface{}) Responder {

	// ensure null doesn't go to client side
	if reflect.ValueOf(list).IsNil() {
		list = []interface{}{}
	}

	return Ok(list)
}

func AppError(str string) Responder {
	return &responder{
		status: http.StatusInternalServerError,
		data:   errorData(str, "", "", nil),
	}
}

func Accepted(status int, data interface{}) Responder {
	return &responder{
		status: status,
		data:   data,
	}
}

func BadRequest(str string) Responder {
	return &responder{
		status: http.StatusBadRequest,
		data:   errorData(str, "", "", nil),
	}
}

func Redirect(url string) Responder {
	return &freeformResponder{
		respond: func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, url, http.StatusFound)
		},
	}
}

func InternalServerError(str string) Responder {
	return &responder{
		status: http.StatusInternalServerError,
		data:   errorData(str, "", "", nil),
	}
}

func UnProcessable() Responder {
	return &responder{
		status: http.StatusUnprocessableEntity,
		data:   errorData("unable to process that request", "", "", nil),
	}
}

func NotFound(msg ...string) Responder {
	return &responder{
		status: http.StatusNotFound,
		data:   strings.Join(msg, " "),
	}
}

func Todo() Responder {
	return WithCodeAndMessage(500, "todo")
}

func WithCodeAndMessage(code int, message string) Responder {
	return &responder{
		status: code,
		data:   message,
	}
}

func WithCode(code int) Responder {
	return &responder{
		status: code,
	}
}

func NotAuthorized(reason ...string) Responder {

	msg := "not authorized"
	if len(reason) > 0 {
		msg += ": " + strings.Join(reason, ", ")
	}

	return &responder{
		status: http.StatusUnauthorized,
		data:   errorData("not authorized", "", msg, reason),
	}
}

func TooMayRequests() Responder {
	return WithCodeAndMessage(http.StatusTooManyRequests, "Too many requests")
}

func ShowBasicAuthPrompt(message string) Responder {
	return &freeformResponder{
		respond: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("WWW-Authenticate", `Basic realm="`+message+`"`)
			w.WriteHeader(401)
			w.Write([]byte("Unauthorised.\n"))
		},
	}
}

func errorData(str string, stackTrace string, msg string, details any) interface{} {
	return map[string]interface{}{
		"error":      str,
		"message":    msg,
		"stackTrace": stackTrace,
		"details":    details,
	}
}
