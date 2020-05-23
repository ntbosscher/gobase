package res

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"reflect"
	"strings"
	"time"
)

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

	if err := json.NewEncoder(w).Encode(resp.data); err != nil {
		log.Println(err)
	}
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
			w.Header().Add("Content-Disposition", "attachment; filename="+sanitizeDispositionName(name)+"")
			http.ServeContent(w, r, name, time.Now(), data)
		},
	}
}

// there's a chrome bug that doesn't handle commas in Content-Disposition filenames
// https://answers.nuxeo.com/general/q/d8348e07fe5e441183bae07dfda00e40/Comma-in-file-name-cause-problem-in-Chrome-Browser
func sanitizeDispositionName(fileName string) string {
	return strings.Replace(fileName, ",", "", -1)
}

func Display(name string, data io.ReadSeeker) Responder {
	return &freeformResponder{
		respond: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Content-Disposition", "inline; filename="+sanitizeDispositionName(name)+"")
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

	return &responder{
		status: http.StatusOK,
		data:   dt,
	}
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
		status: http.StatusOK,
		data:   errorData(str),
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
		data:   errorData(str),
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
		data:   errorData(str),
	}
}

func UnProcessable() Responder {
	return &responder{
		status: http.StatusUnprocessableEntity,
		data:   errorData("unable to process that request"),
	}
}

func NotAuthorized() Responder {
	return &responder{
		status: http.StatusUnauthorized,
		data:   errorData("not authorized"),
	}
}

func errorData(str string) interface{} {
	return map[string]interface{}{
		"error": str,
	}
}
