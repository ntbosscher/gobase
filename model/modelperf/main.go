package modelperf

import (
	"context"
	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/model"
	"sort"
	"sync"
	"time"
)

type Perf struct {
	rows []*Record
	mu   sync.RWMutex
}

func (p *Perf) GetRecords() []*Record {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.rows
}

type Summary struct {
	Method          string
	Query           string
	CallCount       int
	ErrorCount      int
	AverageDuration time.Duration
	TotalDuration   time.Duration
	FirstSeen       time.Time
}

type ByTotalDuration []*Summary

func (b ByTotalDuration) Len() int      { return len(b) }
func (b ByTotalDuration) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b ByTotalDuration) Less(i, j int) bool {
	return b[i].TotalDuration < b[j].TotalDuration
}

type ByAverageDuration []*Summary

func (b ByAverageDuration) Len() int      { return len(b) }
func (b ByAverageDuration) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b ByAverageDuration) Less(i, j int) bool {
	return b[i].AverageDuration < b[j].AverageDuration
}

type ByCallCount []*Summary

func (b ByCallCount) Len() int      { return len(b) }
func (b ByCallCount) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b ByCallCount) Less(i, j int) bool {
	return b[i].CallCount < b[j].CallCount
}

func (p *Perf) GetSummaries() []*Summary {
	p.mu.RLock()
	defer p.mu.RUnlock()

	mapped := map[string][]*Record{}

	for _, row := range p.rows {
		key := row.Method + "." + row.Query
		mapped[key] = append(mapped[key], row)
	}

	var list []*Summary

	for _, v := range mapped {

		sum := &Summary{
			Method:    v[0].Method,
			Query:     v[0].Query,
			CallCount: len(v),
		}

		for _, rec := range v {
			if rec.Error != nil {
				sum.ErrorCount++
			}

			dur := rec.End.Sub(rec.Start)
			sum.TotalDuration += dur
		}

		if sum.CallCount > 0 {
			sum.AverageDuration = sum.TotalDuration / time.Duration(sum.CallCount)
		}

		list = append(list, sum)
	}

	sort.Sort(ByTotalDuration(list))

	return list
}

type Record struct {
	Method string
	Query  string
	Args   []interface{}
	Start  time.Time
	End    time.Time
	Error  error
}

type PerfInput struct {
	// Filter determines which records to keep (true=keep)
	// pass nil to keep all
	Filter func(r *Record) bool
}

// NewPerf creates a new performance tracker
// use the context-cancel func to clean up resources when
func NewPerf(ctx context.Context, input *PerfInput) (context.Context, context.CancelFunc, *Perf) {
	p := &Perf{}

	inputC := make(chan *Record, 1)
	ctx, cancel := context.WithCancel(ctx)

	ctx = model.SetHook(ctx, func(ctx context.Context, method string, query string, args []interface{}, resultError error, start time.Time, end time.Time) {
		rec := &Record{
			Query: query,
			Args:  args,
			Start: start,
			End:   end,
			Error: resultError,
		}

		if input.Filter != nil && !input.Filter(rec) {
			return
		}

		select {
		case inputC <- rec:
		case <-ctx.Done():
			er.Throw("context cancelled")
		}
	})

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case v := <-inputC:
				p.mu.Lock()
				p.rows = append(p.rows, v)
				p.mu.Unlock()
			}
		}
	}()

	return ctx, cancel, p
}
