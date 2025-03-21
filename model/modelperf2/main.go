package modelperf2

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/ntbosscher/gobase/env"
	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/jv"
	"github.com/ntbosscher/gobase/model/modelperf"
)

var runPerf = env.OptionalBool("DEBUG_PERF", false)
var runPerfLogFile = env.Optional("DEBUG_PERF_LOG_FILE", "/tmp/perf.txt")
var captureAllPerf = env.OptionalBool("DEBUG_PERF_ALL", false)

func PerfMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !runPerf {
			next.ServeHTTP(w, req)
			return
		}

		capture := captureAllPerf || req.Header.Get("X-Debug-Perf") == ""
		if !capture {
			next.ServeHTTP(w, req)
			return
		}

		ctx, cancel, perf := modelperf.New(req.Context(), &modelperf.PerfInput{})
		defer cancel()

		req = req.WithContext(ctx)
		next.ServeHTTP(w, req)

		sum := perf.GetSummaries()

		urlCopy, _ := url.Parse(req.URL.String())
		urlCopy.RawQuery = ""

		select {
		case perfC <- &PerfInfo{
			Request: req.Method + " " + urlCopy.String(),
			Info:    sum,
		}:
		default:
		}
	})
}

type PerfInfo struct {
	Request string
	Info    []*modelperf.Summary
}

var perfC = make(chan *PerfInfo, 100)

func init() {
	if !runPerf {
		return
	}

	go func() {
		defer er.HandleErrors(func(input *er.HandlerInput) {
			log.Println(input)
		})

		merged := map[string][]*PerfInfo{}
		tc := time.NewTicker(5 * time.Second)

		defer tc.Stop()

		changed := false

		for {
			select {
			case p := <-perfC:
				merged[p.Request] = append(merged[p.Request], p)
				changed = true
			case <-tc.C:
				if !changed {
					continue
				}

				changed = false
				printPerfMap(merged)
			}
		}
	}()
}

func ellipsis(qr string, max int) string {
	if len(qr) <= max {
		return qr
	}

	return qr[:max] + "..."

}

func printPerfMap(merged map[string][]*PerfInfo) {
	buf := &bytes.Buffer{}

	queries := map[string]string{}

	list := jv.GetMapValues(merged)

	list = jv.Reverse(jv.SortFx(list, func(a []*PerfInfo) float64 {
		times := jv.Mapper(a, func(a *PerfInfo) time.Duration {
			sum := time.Duration(0)

			for _, item := range a.Info {
				sum += item.TotalDuration
			}

			return sum
		})

		worstTime := time.Duration(0)
		for _, tm := range times {
			if tm > worstTime {
				worstTime = tm
			}
		}

		return float64(worstTime)
	}))

	for _, infos := range list {
		buf.WriteString("------------------------------------------------------\n")
		buf.WriteString(infos[0].Request + "\n")

		have := map[string]bool{}

		sum := []*modelperf.Summary{}
		for _, item := range infos {
			sum = append(sum, item.Info...)
		}

		sum = jv.Reverse(jv.SortFx(sum, func(a *modelperf.Summary) float64 {
			return float64(a.TotalDuration)
		}))

		for _, qr := range sum {
			if have[qr.Query] {
				continue
			}

			fmt.Fprintln(buf, ellipsis(qr.Query, 200))
			hash := md5.Sum([]byte(qr.Query))
			hashStr := hex.EncodeToString(hash[:])
			fmt.Fprintln(buf, hashStr)
			queries[hashStr] = qr.Query

			for _, item := range sum {
				if item.Query == qr.Query {
					fmt.Fprintln(buf, "avg", qr.AverageDuration.String(), "count", qr.CallCount, "total", qr.TotalDuration.String())
				}
			}

			have[qr.Query] = true
		}
	}

	for hash, item := range queries {
		fmt.Fprintln(buf, "------------------------------------------------------")
		fmt.Fprintln(buf, hash)
		fmt.Fprintln(buf, item)
	}

	os.WriteFile(runPerfLogFile, buf.Bytes(), 0644)
}
