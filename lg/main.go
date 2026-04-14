package lg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ntbosscher/gobase/randomish"
)

var queue chan *message

func init() {
	queue = make(chan *message, 100)
	go writer()
}

type message struct {
	sender *LogSender
	when   time.Time
	str    []byte
	caller string
	extra  map[string]any
}

type LogSender struct {
	key       string
	start     time.Time
	parent    string
	startMeta map[string]any

	hasLogged *atomic.Bool
}

func (s *LogSender) SetMeta(key string, value string) {
	s.startMeta[key] = value
}

func (s *LogSender) GetKey() string {
	return s.key
}

type logContextKeyType string

var logContextKey = logContextKeyType("lg-context-key")

type Option func(*LogSender)

func OptWithParent(parent string) Option {
	return func(s *LogSender) {
		s.parent = parent
		s.startMeta["parent"] = parent
	}
}

func TraceKey(ctx context.Context) string {
	obj := get(ctx)
	if obj == nil {
		return "<nil>"
	}

	if obj.hasLogged.CompareAndSwap(false, true) {
		Println(ctx, "extracting trace-key for background job")
	}

	return obj.key
}

func NewContext(ctx context.Context, opts ...Option) context.Context {
	send := get(ctx)
	if send != nil {
		for _, opt := range opts {
			opt(send)
		}

		return ctx
	}

	start := time.Now()
	send = &LogSender{
		key:       makeKey(ctx, start),
		hasLogged: &atomic.Bool{},
		startMeta: map[string]any{
			"start": start.Format(time.RFC3339),
		},
	}

	send.hasLogged.Store(false)

	for _, opt := range opts {
		opt(send)
	}

	return context.WithValue(ctx, logContextKey, send)
}

func OptWithRequest(req *http.Request) Option {
	return func(s *LogSender) {
		s.startMeta["req:method"] = req.Method
		s.startMeta["req:url"] = req.URL.String()
	}
}

type HttpMiddlewareOpt func(w http.ResponseWriter, req *http.Request, send *LogSender)

func HttpMiddleware(httpMiddlewareOpt ...HttpMiddlewareOpt) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := NewContext(req.Context(), OptWithRequest(req))
			req = req.WithContext(ctx)

			if len(httpMiddlewareOpt) > 0 {
				sender := get(ctx)

				for _, opt := range httpMiddlewareOpt {
					opt(w, req, sender)
				}
			}

			next.ServeHTTP(w, req)
		})
	}
}

func get(ctx context.Context) *LogSender {
	s, ok := ctx.Value(logContextKey).(*LogSender)
	if !ok {
		return nil
	}

	return s
}

func makeKey(ctx context.Context, start time.Time) string {
	return randomish.GetAlphaNumericChars(6) + "-" + start.Format("0405.000")
}

func Println(ctx context.Context, v ...any) {
	sender := get(ctx)
	checkSenderInit(ctx, sender)

	buf := &bytes.Buffer{}

	for i, value := range v {
		buf.WriteString(fmt.Sprint(value))
		if i > 0 && len(v) > i+1 {
			buf.WriteString(" ")
		}
	}

	select {
	case queue <- &message{
		when:   time.Now(),
		sender: get(ctx),
		caller: getCaller(2),
		str:    buf.Bytes(),
	}:
	case <-ctx.Done():
	}
}

func checkSenderInit(ctx context.Context, sender *LogSender) {
	if sender == nil {
		return
	}

	if !sender.hasLogged.CompareAndSwap(false, true) {
		return
	}

	select {
	case queue <- &message{
		when:   time.Now(),
		sender: sender,
		caller: getCaller(3),
		str:    []byte("trace-origin"),
		extra:  sender.startMeta,
	}:
	case <-ctx.Done():
	}
}

func PrintObj(ctx context.Context, str string, v map[string]any) {
	sender := get(ctx)
	checkSenderInit(ctx, sender)

	select {
	case queue <- &message{
		when:   time.Now(),
		sender: sender,
		caller: getCaller(2),
		str:    []byte(str),
		extra:  v,
	}:
	case <-ctx.Done():
	}
}

func getCaller(callDepth int) string {
	var ok bool
	_, file, line, ok := runtime.Caller(callDepth)
	if !ok {
		file = "???"
		line = 0
	}

	dir, baseFile := filepath.Split(file)
	dirName := filepath.Base(dir)

	return fmt.Sprintf("%s:%d", filepath.Join(dirName, baseFile), line)
}

func writer() {
	defer log.Println("lg: writer exited")

	for item := range queue {
		write(item)
	}
}

func toJson(v any) []byte {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	err := enc.Encode(v)
	if err != nil {
		log.Println("lg: write error: json", err)
	}

	content := buf.Bytes()
	if len(content) > 0 {
		return content[0 : len(content)-1] // remove trailing newline
	}

	return content
}

const senderNil = "<nil>"

func write(input *message) {
	defer func() {
		if err := recover(); err != nil {
			log.Println("lg: write panic:", err)
		}
	}()

	parent := ""
	senderKey := senderNil

	if input.sender != nil {
		senderKey = input.sender.key
		parent = input.sender.parent
	}

	row := fmt.Sprintf(`{"d":"%s","t":"%s","s":%s,"e":%s,"f":"%s","pr":"%s"}`,
		input.when.Format("2006-01-02T15:04:05"),
		senderKey,
		string(toJson(string(input.str))),
		string(toJson(input.extra)),
		input.caller,
		parent)

	row = row + "\n"
	_, err := io.Copy(os.Stdout, strings.NewReader(row))
	if err != nil {
		log.Println("lg: write error:", err)
	}
}
