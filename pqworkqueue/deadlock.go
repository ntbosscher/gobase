package pqworkqueue

import (
	"context"
	"database/sql"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/model"
	"github.com/ntbosscher/gobase/pqshared"
	"github.com/pkg/errors"
)

// The deadlock detector breaks deadlocks that pass from a job's claim transaction to its callback
// transaction (see startWork), which Postgres can't detect. It cancels the callback transaction of
// one job, so that job errors out.
//
// The detector only knows the claim and callback transactions of each job. It can't break a
// deadlock through other database work a callback does.

// DeadlockCheckInterval is how often the detector checks for deadlocks.
var DeadlockCheckInterval = 5 * time.Second

// PoolStallTimeout is how long the model connection pool can stay full while jobs wait for their
// callback connection, and no waiting job gets one, before the detector treats it as a deadlock.
var PoolStallTimeout = 1 * time.Minute

// DeadlockRetryDelay is how long a job cancelled by ErrDeadlockPoolExhausted waits before it runs again.
var DeadlockRetryDelay = 10 * time.Second

const deadlockQueryTimeout = 10 * time.Second

var ErrDeadlockLockWait = errors.New("pqworkqueue: deadlock detected: job callback waits on a lock held by a job's claim transaction")
var ErrDeadlockPoolExhausted = errors.New("pqworkqueue: deadlock detected: connection pool exhausted while job waits for its callback connection")

var detector = &deadlockDetector{
	jobs: map[string]*activeJob{},
}

type activeJob struct {
	id           string
	waitingSince time.Time
	running      bool
	cancelled    bool
	cancel       context.CancelCauseFunc
}

type deadlockDetector struct {
	mu   sync.Mutex
	jobs map[string]*activeJob

	lastProgress time.Time
}

func (d *deadlockDetector) add(id string, cancel context.CancelCauseFunc) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.jobs[id] = &activeJob{
		id:           id,
		waitingSince: time.Now(),
		cancel:       cancel,
	}
}

// setRunning records that the job's callback transaction has its connection.
// It returns false if the detector already cancelled the job.
func (d *deadlockDetector) setRunning(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	job := d.jobs[id]
	if job.cancelled {
		return false
	}

	job.running = true
	d.lastProgress = time.Now()
	return true
}

func (d *deadlockDetector) remove(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.jobs, id)
}

// publish records the job's transactions in pq_worker_active, so the lock-wait check of every
// process can follow them. It returns false if the record failed.
func (d *deadlockDetector) publish(id string, outerPid int32, innerPid int32) bool {
	ctx, cancel := context.WithTimeout(context.Background(), deadlockQueryTimeout)
	defer cancel()

	_, err := pqshared.Pool.Exec(ctx, `
		insert into pq_worker_active (job_id, outer_pid, outer_xact_start, inner_pid, inner_xact_start)
		select $1, o.pid, o.xact_start at time zone 'utc', i.pid, i.xact_start at time zone 'utc'
		from pg_stat_activity o, pg_stat_activity i
		where o.pid = $2 and i.pid = $3
		on conflict (job_id) do update set
			outer_pid = excluded.outer_pid,
			outer_xact_start = excluded.outer_xact_start,
			inner_pid = excluded.inner_pid,
			inner_xact_start = excluded.inner_xact_start
	`, id, outerPid, innerPid)

	if err != nil {
		Logger.Println("failed to publish active job", id, err)
		return false
	}

	return true
}

func (d *deadlockDetector) unpublish(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), deadlockQueryTimeout)
	defer cancel()

	_, err := pqshared.Pool.Exec(ctx, `delete from pq_worker_active where job_id = $1`, id)
	if err != nil {
		Logger.Println("failed to remove active job", id, err)
	}
}

func (d *deadlockDetector) cancelLocked(job *activeJob, cause error) {
	Logger.Println("cancelling job", job.id, cause)
	job.cancelled = true
	job.cancel(cause)
}

func (d *deadlockDetector) run() {
	for {
		<-time.After(DeadlockCheckInterval)
		d.check()
	}
}

func (d *deadlockDetector) check() {
	defer er.HandleErrors(func(input *er.HandlerInput) {
		Logger.Println(input.Error, input.StackTrace)
	})

	d.checkPoolExhausted()
	d.checkLockWait()
}

func (d *deadlockDetector) checkPoolExhausted() {
	stats := model.Stats(context.Background())
	now := time.Now()

	d.mu.Lock()
	defer d.mu.Unlock()

	var waiting []*activeJob
	for _, job := range d.jobs {
		if !job.running && !job.cancelled {
			waiting = append(waiting, job)
		}
	}

	full := stats.MaxOpenConnections > 0 && stats.InUse >= stats.MaxOpenConnections
	if !full || len(waiting) == 0 {
		d.lastProgress = now
		return
	}

	oldest := slices.MinFunc(waiting, func(a, b *activeJob) int {
		return a.waitingSince.Compare(b.waitingSince)
	})

	if now.Sub(d.lastProgress) < PoolStallTimeout || now.Sub(oldest.waitingSince) < PoolStallTimeout {
		return
	}

	d.cancelLocked(oldest, ErrDeadlockPoolExhausted)
	d.lastProgress = now
}

func (d *deadlockDetector) checkLockWait() {
	var ids []string

	d.mu.Lock()
	for _, job := range d.jobs {
		if job.running && !job.cancelled {
			ids = append(ids, job.id)
		}
	}
	d.mu.Unlock()

	if len(ids) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), deadlockQueryTimeout)
	defer cancel()

	// Walk the Postgres lock waits out from the callback transaction of each local job. When a wait
	// reaches the claim transaction of any active job (of any process), continue from that job's
	// callback transaction (the wait Postgres can't see). A job is deadlocked when the walk reaches
	// its own claim transaction. Only the job with the lowest id in a cycle is selected, so one side
	// errors. Rows count only while both of their transactions are still open, so reused pids are ignored.
	// Use pqshared.Pool so the check still runs when the model pool is exhausted.
	id := ""
	err := pqshared.Pool.QueryRow(ctx, `
		with recursive hops as (
			select w.job_id as id, w.outer_pid, w.inner_pid
			from pq_worker_active w
			join pg_stat_activity o on o.pid = w.outer_pid and o.xact_start at time zone 'utc' = w.outer_xact_start
			join pg_stat_activity i on i.pid = w.inner_pid and i.xact_start at time zone 'utc' = w.inner_xact_start
		), waits(id, outer_pid, pid, min_id) as (
			select j.id, j.outer_pid, b.pid, j.id
			from hops j
			cross join lateral unnest(pg_blocking_pids(j.inner_pid)) as b(pid)
			where j.id = any($1::text[])
			union
			select w.id, w.outer_pid, b.pid, least(w.min_id, hop.id)
			from waits w
			left join hops hop on hop.outer_pid = w.pid
			cross join lateral unnest(pg_blocking_pids(coalesce(hop.inner_pid, w.pid))) as b(pid)
		)
		select id
		from waits
		where pid = outer_pid and id = min_id
		order by id
		limit 1
	`, ids).Scan(&id)

	if err != nil && errors.Is(err, sql.ErrNoRows) {
		return
	}

	er.Check(err)

	if !d.cancelLockWait(id) {
		return
	}

	// Cancel the blocked statement on the server too, in case the context cancel doesn't reach it.
	_, err = pqshared.Pool.Exec(ctx, `
		select pg_cancel_backend(w.inner_pid)
		from pq_worker_active w
		join pg_stat_activity i on i.pid = w.inner_pid and i.xact_start at time zone 'utc' = w.inner_xact_start
		where w.job_id = $1
	`, id)
	er.Check(err)
}

func (d *deadlockDetector) cancelLockWait(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	job := d.jobs[id]
	if job == nil || !job.running || job.cancelled {
		return false
	}

	d.cancelLocked(job, ErrDeadlockLockWait)
	return true
}

// jobLimit caps in-flight jobs so they hold at most all but 2 connections of the model pool, since
// each job holds up to 2 connections.
var jobLimit = newJobLimiter()

type jobLimiter struct {
	mu     sync.Mutex
	cond   *sync.Cond
	active int
}

func newJobLimiter() *jobLimiter {
	l := &jobLimiter{}
	l.cond = sync.NewCond(&l.mu)
	return l
}

func (l *jobLimiter) limit() int {
	maxOpen := model.Stats(context.Background()).MaxOpenConnections
	if maxOpen <= 0 {
		return math.MaxInt
	}

	return max(1, (maxOpen-2)/2)
}

func (l *jobLimiter) acquire() {
	l.mu.Lock()
	defer l.mu.Unlock()

	for l.active >= l.limit() {
		l.cond.Wait()
	}

	l.active++
}

func (l *jobLimiter) release() {
	l.mu.Lock()
	l.active--
	l.mu.Unlock()

	l.cond.Broadcast()
}
