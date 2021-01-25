package res

import (
	jsoniter "github.com/json-iterator/go"
	"github.com/json-iterator/go/extra"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"reflect"
	"strings"
	"time"
	"unicode"
)

var json = jsoniter.ConfigDefault
var errorLogger *log.Logger
var ignoreErrorLog ErrorLogFilterer

type ErrorLogFilterer func(responseCode int, responseBody interface{}, request *http.Request) bool

func init() {
	extra.SetNamingStrategy(jsonRenameKeysToCamelCase)
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

func jsonRenameKeysToCamelCase(key string) string {

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

			js, _ := json.MarshalIndent(resp.data, "", "   ")
			jsStr := string(js)
			if jsStr != `""` {
				jsStr = "\n" + jsStr
			} else {
				jsStr = ""
			}

			errorLogger.Printf("request failed: %s %s -> %d%s", r.Method, r.URL, resp.status, jsStr)
		}
	}

	if err := json.NewEncoder(w).Encode(resp.data); err != nil {
		log.Println(err)
	}
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

func Error(err error) Responder {
	return AppError(err.Error())
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
			w.Header().Add("Content-Disposition", "attachment; filename="+SanitizeDispositionName(name)+"")
			http.ServeContent(w, r, name, time.Now(), data)
		},
	}
}

// there's a chrome bug that doesn't handle commas in Content-Disposition filenames
// https://answers.nuxeo.com/general/q/d8348e07fe5e441183bae07dfda00e40/Comma-in-file-name-cause-problem-in-Chrome-Browser
func SanitizeDispositionName(fileName string) string {
	return strings.Replace(fileName, ",", "", -1)
}

func Display(name string, data io.ReadSeeker) Responder {
	return &freeformResponder{
		respond: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Content-Disposition", "inline; filename="+SanitizeDispositionName(name)+"")
			http.ServeContent(w, r, name, time.Now(), data)
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
		data:   errorData(str, "", ""),
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
		data:   errorData(str, "", ""),
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
		data:   errorData(str, "", ""),
	}
}

func UnProcessable() Responder {
	return &responder{
		status: http.StatusUnprocessableEntity,
		data:   errorData("unable to process that request", "", ""),
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

func NotAuthorized(reason ...string) Responder {

	msg := "not authorized"
	if len(reason) > 0 {
		msg += ": " + strings.Join(reason, ", ")
	}

	return &responder{
		status: http.StatusUnauthorized,
		data:   errorData("not authorized", "", msg),
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

func errorData(str string, stackTrace string, msg string) interface{} {
	return map[string]interface{}{
		"error":      str,
		"message":    msg,
		"stackTrace": stackTrace,
	}
}
