// Package pqprogress lets a pqworkqueue job report its progress (a description and a percent) to
// the page that waits for it. It is optional: a queue uses it only when it registers Middleware.
//
//	queue.RegisterWorker(1, worker, pqprogress.Middleware())
//
//	func worker(ctx context.Context, arg *Input) []byte {
//		pqprogress.Set(ctx, "Copying rows", 25)
//		...
//	}
//
// The page reads the report with Get(ctx, jobID) while it polls the job status.
//
// A job's work runs in its own transaction, so its writes are not visible until it commits. The
// reports are therefore written on pqshared.Pool, one autocommit statement each, outside the job's
// transaction. No nested transaction is opened on the job's ctx (see UPGRADING.md, v0.13.0).
//
// Set never waits for the database. Each job has one writer goroutine that writes the latest report.
// Reports that come faster than the writer can write are merged: only the latest one is written.
//
// The table pq_worker_progress is created on init. With PQWORKQUEUE_SKIP_MIGRATE=true, the app must
// create it itself (see the statement in init).
package pqprogress

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/ntbosscher/gobase/env"
	"github.com/ntbosscher/gobase/pqshared"
	"github.com/ntbosscher/gobase/pqworkqueue"
)

var Logger = log.New(os.Stderr, "pqprogress", log.Llongfile)

// WriteInterval limits the writes for one job. A report with a new description, or with percent 100,
// is never held back.
var WriteInterval = 500 * time.Millisecond

// WriteTimeout limits one write or delete, so that a writer goroutine cannot wait forever.
var WriteTimeout = 10 * time.Second

// StaleAfter is the age of a report that init deletes: a job that ended without its middleware
// cleanup (e.g. a crash).
var StaleAfter = 24 * time.Hour

func init() {
	if env.OptionalBool("PQWORKQUEUE_SKIP_MIGRATE", false) {
		return
	}

	_, err := pqshared.Pool.Exec(context.Background(), `create table if not exists pq_worker_progress (
		id text not null primary key,
		description text not null,
		percent real not null,
		updated_at timestamp not null
	);`)
	if err != nil {
		log.Fatal("failed to setup worker progress table: ", err)
	}

	_, err = pqshared.Pool.Exec(context.Background(), `delete from pq_worker_progress where updated_at < $1`,
		time.Now().UTC().Add(-StaleAfter))
	if err != nil {
		Logger.Println("failed to delete stale progress:", err)
	}
}

// Progress is the last report of a job.
type Progress struct {
	Description string    `json:"description"`
	Percent     float64   `json:"percent"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type ctxKey struct{}

type reporter struct {
	id   string
	wake chan struct{}
	stop chan struct{}

	mu      sync.Mutex
	pending *Progress
	stopped bool
}

func newReporter(id string) *reporter {
	return &reporter{
		id:   id,
		wake: make(chan struct{}, 1),
		stop: make(chan struct{}),
	}
}

// Middleware gives each job of the queue a reporter for its job id, and deletes the report when the
// job ends. Pass it to RegisterWorker.
func Middleware() func(*pqworkqueue.WorkerInfo) {
	return func(info *pqworkqueue.WorkerInfo) {
		info.Middleware = append(info.Middleware, func(next pqworkqueue.Worker) pqworkqueue.Worker {
			return func(ctx context.Context, input json.RawMessage, meta pqworkqueue.WorkerJobMeta) []byte {
				r := newReporter(meta.ID)
				go r.run()
				defer r.close()

				return next(context.WithValue(ctx, ctxKey{}, r), input, meta)
			}
		})
	}
}

// Set reports the progress of the job of ctx. percent is clamped to 0..100. Without Middleware it
// does nothing. It does not wait for the write, and a failed write only logs: progress must not fail
// or slow down the job.
func Set(ctx context.Context, description string, percent float64) {
	r, _ := ctx.Value(ctxKey{}).(*reporter)
	if r == nil {
		return
	}

	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return
	}

	r.pending = &Progress{
		Description: description,
		Percent:     min(max(percent, 0), 100),
		UpdatedAt:   time.Now().UTC(),
	}
	r.mu.Unlock()

	select {
	case r.wake <- struct{}{}:
	default:
	}
}

func (r *reporter) take() *Progress {
	r.mu.Lock()
	defer r.mu.Unlock()

	p := r.pending
	r.pending = nil
	return p
}

func (r *reporter) close() {
	r.mu.Lock()
	r.stopped = true
	r.mu.Unlock()

	close(r.stop)
}

// run writes the reports of one job until the job ends, then deletes its row.
func (r *reporter) run() {
	attempted := false
	lastWrite := time.Time{}
	lastDescription := ""

	defer func() {
		if attempted {
			r.remove()
		}
	}()

	for {
		select {
		case <-r.stop:
			return
		default:
		}

		select {
		case <-r.wake:
		case <-r.stop:
			return
		}

		p := r.take()
		if p == nil {
			continue
		}

		urgent := !attempted || p.Description != lastDescription || p.Percent >= 100
		if !urgent {
			select {
			case <-time.After(time.Until(lastWrite.Add(WriteInterval))):
			case <-r.stop:
				return
			}

			// a newer report came in during the wait
			if newer := r.take(); newer != nil {
				p = newer
			}
		}

		attempted = true
		lastWrite = time.Now()
		lastDescription = p.Description
		r.write(p)
	}
}

func (r *reporter) write(p *Progress) {
	ctx, cancel := context.WithTimeout(context.Background(), WriteTimeout)
	defer cancel()

	_, err := pqshared.Pool.Exec(ctx, `
		insert into pq_worker_progress (id, description, percent, updated_at)
		values ($1, $2, $3, $4)
		on conflict (id) do update
		set description = excluded.description, percent = excluded.percent, updated_at = excluded.updated_at
	`, r.id, p.Description, p.Percent, p.UpdatedAt)
	if err != nil {
		Logger.Println("failed to write progress:", r.id, err)
	}
}

func (r *reporter) remove() {
	ctx, cancel := context.WithTimeout(context.Background(), WriteTimeout)
	defer cancel()

	_, err := pqshared.Pool.Exec(ctx, `delete from pq_worker_progress where id = $1`, r.id)
	if err != nil {
		Logger.Println("failed to delete progress:", r.id, err)
	}
}

// Get is the last report of the job, nil when it has none.
func Get(ctx context.Context, jobID string) (*Progress, error) {
	p := &Progress{}
	err := pqshared.Pool.QueryRow(ctx, `
		select description, percent, updated_at
		from pq_worker_progress
		where id = $1
	`, jobID).Scan(&p.Description, &p.Percent, &p.UpdatedAt)
	if err != nil && errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return p, nil
}
