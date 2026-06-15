package pqworkqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gobuffalo/nulls"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/ntbosscher/gobase/env"
	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/model"
	"github.com/ntbosscher/gobase/model/modelutil"
	"github.com/ntbosscher/gobase/pqshared"
	"github.com/pkg/errors"
)

var addListen chan *WorkerInfo
var Logger = log.New(os.Stderr, "pqworkerqueue", log.Llongfile)
var Debug = log.New(os.Stdout, "pqworkerqueue", log.Llongfile)
var DebugPrintJobStats = false
var FallbackCheckInterval = 5 * time.Minute

func init() {

	addListen = make(chan *WorkerInfo)
	var err error

	DebugPrintJobStats = env.OptionalBool("PQWORKQUEUE_PRINT_JOB_STATS", false)
	skipAll := env.OptionalBool("PQWORKQUEUE_SKIP_MIGRATE", false)
	skipThis := env.Optional("PQWORKQUEUE_MIGRATION_LEVEL", "") == "2026-June-01"

	needsMigration := !skipAll
	if skipThis {
		needsMigration = false
	}

	if needsMigration {
		_, err = pqshared.Pool.Exec(context.Background(), `create table if not exists pq_worker_queue (
			id text not null unique,
			queue_name text not null,
			debounce_key text not null,
			job_arg json not null,
			result bytea null,
			created_at timestamp not null,
			start_after timestamp not null,
			started_at timestamp null,
			completed_at timestamp null,
			retain_until timestamp null,
			commit_error text null
		);`)
		if err != nil {
			log.Fatal("failed to setup worker table: ", err)
		}

		_, err = pqshared.Pool.Exec(context.Background(), `alter table pq_worker_queue 
    		add column if not exists debounce_key text not null default '',
    		add column if not exists start_after timestamp not null default current_timestamp,
    		add column if not exists commit_error text null;`)
		if err != nil {
			log.Fatal("failed to setup worker table: ", err)
		}

		_, err = pqshared.Pool.Exec(context.Background(), `create index if not exists ix_pq_worker_queue_retain on pq_worker_queue (retain_until);`)
		if err != nil {
			log.Fatal("failed to setup worker table: ", err)
		}

		// remove old index: create index if not exists ix_pq_worker_queue_pending on pq_worker_queue (queue_name, start_after) where started_at is not null;
		_, err = pqshared.Pool.Exec(context.Background(), `drop index if exists ix_pq_worker_queue_pending;`)
		if err != nil {
			log.Fatal("failed to setup worker table: ", err)
		}

		_, err = pqshared.Pool.Exec(context.Background(), `create index if not exists ix_pq_worker_queue_pending2 on pq_worker_queue (queue_name, start_after) where started_at is null;`)
		if err != nil {
			log.Fatal("failed to setup worker table: ", err)
		}

		// no longer want unique index
		_, err = pqshared.Pool.Exec(context.Background(), `drop index if exists ix_pq_worker_debounce;`)
		if err != nil {
			log.Fatal("failed to setup worker table: ", err)
		}

		_, err = pqshared.Pool.Exec(context.Background(), `create index if not exists ix_pq_worker_queue_id on pq_worker_queue (id);`)
		if err != nil {
			log.Fatal("failed to setup worker table: ", err)
		}
	}

	go cleaner()
	go watcher()
}

type watcherInfo struct {
	muListeningFor          sync.RWMutex
	listeningFor            map[string]*WorkerInfo
	defaultTxIsolationLevel sql.IsolationLevel

	signal chan string
}

func watcher() {
	er.HandleErrors(func(input *er.HandlerInput) {
		log.Println(input.Error, input.StackTrace)
	})

	defaultTxIsolationLevel := env.OptionalInt("PQ_WORKER_QUEUE_TX_ISOLATION_LEVEL", int(sql.LevelDefault))
	switch sql.IsolationLevel(defaultTxIsolationLevel) {
	case sql.LevelDefault:
	case sql.LevelReadUncommitted:
	case sql.LevelReadCommitted:
	case sql.LevelWriteCommitted:
	case sql.LevelRepeatableRead:
	case sql.LevelSnapshot:
	case sql.LevelSerializable:
	case sql.LevelLinearizable:
	default:
		log.Fatal("unknown isolation level set via env PQ_WORKER_QUEUE_TX_ISOLATION_LEVEL: ", defaultTxIsolationLevel)
	}

	w := &watcherInfo{
		listeningFor:            map[string]*WorkerInfo{},
		defaultTxIsolationLevel: sql.IsolationLevel(defaultTxIsolationLevel),

		signal: make(chan string, 1),
	}

	go w.addNewListeners()
	go w.monitorScheduled()
	go w.monitorDbNotifications()

	wait := sync.WaitGroup{}
	wait.Add(3)
	wait.Wait()
}

func (w *watcherInfo) processWork(queueName string) {
	defer er.HandleErrors(func(input *er.HandlerInput) {
		Logger.Println(input.Error, input.StackTrace)
	})

	for {
		ok, isolationLevel, callback, middleware, retainResultsFor, delayCallback, done := w.startWorkConcurrencyCheck(queueName)
		if !ok {
			// either we don't handle this queue or we're at the concurrency limit
			return
		}

		claimed := make(chan bool, 1)

		go func() {
			defer er.HandleErrors(func(input *er.HandlerInput) {
				Logger.Println(input.Error, input.StackTrace)
			})

			defer done()
			didWork := w.startWork(queueName, isolationLevel, callback, middleware, retainResultsFor, delayCallback, claimed)
			if didWork {

				select {
				case w.signal <- queueName:
				default:
				}

			}
		}()

		// Wait only until the job is claimed (a fast DB op), then loop to dispatch the next one.
		// The job's callback runs asynchronously in the goroutine above, so the scheduler
		// goroutine that called processWork (notification listener / timer) is never blocked on
		// job execution. This also lets us actually reach the configured NConcurrent parallelism.
		if !<-claimed {
			return
		}
	}
}

var missingQueueNameLogGate = map[string]time.Time{}
var missingQueueNameLogGateLock sync.Mutex

func checkMissingQueueLogGate(queueName string) bool {
	missingQueueNameLogGateLock.Lock()
	defer missingQueueNameLogGateLock.Unlock()

	value, ok := missingQueueNameLogGate[queueName]
	if ok && time.Since(value) < time.Hour {
		return false
	}

	missingQueueNameLogGate[queueName] = time.Now()
	return true
}

var busyLogGate = map[string]time.Time{}
var busyLogGateLock sync.Mutex

func checkBusyLogGate(queueNames []string) []string {

	busyLogGateLock.Lock()
	defer busyLogGateLock.Unlock()

	var out []string
	for _, item := range queueNames {
		value, ok := busyLogGate[item]

		if ok && time.Since(value) < time.Minute {
			continue
		}

		out = append(out, item)
		busyLogGate[item] = time.Now()
	}

	return out
}

func (w *watcherInfo) startWorkConcurrencyCheck(queueName string) (ok bool, isolationLevel sql.IsolationLevel, callback Worker, middleware []Middleware, retainResultsFor time.Duration, delayCallback DelayForLoadCallback, done func()) {
	w.muListeningFor.RLock()
	defer w.muListeningFor.RUnlock()

	info := w.listeningFor[queueName]
	if info == nil {
		if checkMissingQueueLogGate(queueName) {
			Logger.Println("missing info for queue name", queueName, "you may need to clear this from the `pq_worker_queue` table or check your application")
		}

		// don't have that queue
		return
	}

	done, ok = info.concurrencyCheck()
	if !ok {
		return
	}

	ok = true

	isolationLevel = w.defaultTxIsolationLevel
	if info.TxIsolationLevel != nil {
		isolationLevel = *info.TxIsolationLevel
	}

	callback = info.Callback
	middleware = info.Middleware
	retainResultsFor = info.RetainResultsFor
	delayCallback = info.DelayForLoadCallback
	return
}

// startWork claims and runs a single job. It is expected to run in its own goroutine (so the
// callback never blocks the scheduler goroutine). It signals on `claimed` exactly once: true once
// a job has been claimed (so the caller can dispatch the next one in parallel), or false if no job
// was available (so the caller stops looping). The claim, callback and result-store all happen in a
// single outer transaction, so a crash mid-job rolls back `started_at` and the job is retried.
// It returns ranJob=true if a job was claimed (and therefore a concurrency slot is about to free).
func (w *watcherInfo) startWork(queueName string, isolationLevel sql.IsolationLevel, callback Worker, middleware []Middleware, retainResultsFor time.Duration, delayCallback DelayForLoadCallback, claimed chan bool) (ranJob bool) {

	claimSignalled := false
	signalClaimed := func(v bool) {
		if claimSignalled {
			return
		}
		claimSignalled = true
		claimed <- v
	}

	// Ensure the caller is always released. If we claim a job, signalClaimed(true) below makes this
	// a no-op; otherwise this tells the caller to stop looping.
	defer signalClaimed(false)

	if delayCallback != nil {
		count := 0
		delayStart := time.Now()

		for {
			delayInfo := delayCallback(time.Since(delayStart), count)
			if delayInfo == nil || delayInfo.DelayFor <= 0 {
				break
			}

			<-time.After(delayInfo.DelayFor)

			if !delayInfo.CheckAgainAfterDelay {
				break
			}

			count++
		}
	}

	err := model.WithTx(context.Background(), func(ctx context.Context, tx *sqlx.Tx) error {

		id, message, err := getAndClaimJob(ctx, queueName)
		if err != nil {
			if err != sql.ErrNoRows {
				Logger.Println("failed to get job:", err.Error())
			}

			return err
		}

		// We've claimed a job. Let the scheduler goroutine dispatch the next one in parallel while
		// we run this job's callback below.
		signalClaimed(true)
		ranJob = true

		var result []byte
		tStart := time.Now()

		err2 := model.WithTx2(context.Background(), isolationLevel, func(ctx context.Context, tx *sqlx.Tx) (innerErr error) {
			defer er.HandleErrors(func(input *er.HandlerInput) {
				innerErr = input.Error
			})

			if DebugPrintJobStats {
				Debug.Println("starting job for", "queue:"+queueName, "arg:"+getDebugStringForMessage(message))
			}

			exec := callback
			for _, item := range middleware {
				exec = item(exec)
			}

			result = exec(ctx, id, message)
			return
		})

		if DebugPrintJobStats {
			Debug.Println("finished job for", "queue:"+queueName, "arg:"+getDebugStringForMessage(message), "duration:"+time.Since(tStart).String())
		}

		commitErr := nulls.String{}
		if err2 != nil {
			if errors.Is(err2, model.ErrCommitAlreadyCalled) {
				// already committed is fine
			} else {
				Logger.Println("failed to process job", err2)
				commitErr = nulls.NewString(err2.Error())
			}
		}

		err = model.ExecContext(ctx, `
			update pq_worker_queue set
				result = $1,
				completed_at = $2,
				retain_until = $3,
				commit_error = $4
			where id = $5
		`, result, time.Now().UTC(), time.Now().UTC().Add(retainResultsFor), commitErr, id)

		if err != nil {
			Logger.Println("failed to store result:", err)
		}

		return err
	})

	// On a commit failure the outer transaction rolled back, so `started_at` reverted to null and
	// the job is claimable again. We've already signalled claimed=true, so the caller will loop and
	// re-dispatch it.
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		Logger.Println("failed to commit transaction", err.Error())
	}

	return ranJob
}

func getDebugStringForMessage(content json.RawMessage) string {
	if len(content) < 100 {
		return checkValidCharacters(content)
	}

	return checkValidCharacters(content[:100]) + "..."
}

func checkValidCharacters(value []byte) string {
	out := make([]byte, len(value))

	for i, c := range value {
		if c > ' ' && c < '~' || c == '\n' || c == '\r' || c == '\t' {
			out[i] = c
		} else {
			out[i] = '*'
		}
	}

	return string(out)
}

func getAndClaimJob(ctx context.Context, queueName string) (id string, message json.RawMessage, err error) {

	err = model.QueryRowContext(ctx, `
		select id, job_arg
		from pq_worker_queue
		where queue_name = $1 and start_after <= $2 and started_at is null 
		order by start_after
		for update skip locked
		limit 1
	`, queueName, time.Now().UTC()).Scan(&id, &message)

	if err != nil {
		return
	}

	err = model.ExecContext(ctx, `update pq_worker_queue set started_at = $1 where id = $2`, time.Now().UTC(), id)
	if err != nil {
		return
	}

	return
}

func (w *watcherInfo) addNewListeners() {
	er.HandleErrors(func(input *er.HandlerInput) {
		Logger.Println(input.Error, input.StackTrace)
	})

	for {
		info := <-addListen

		w.muListeningFor.Lock()
		w.listeningFor[info.QueueName] = info
		w.muListeningFor.Unlock()

		select {
		case w.signal <- info.QueueName:
		default:
		}
	}
}

func (w *watcherInfo) monitorScheduled() {
	defer er.HandleErrors(func(input *er.HandlerInput) {
		Logger.Println(input.Error, input.StackTrace)
	})

	nextScheduledTimer := time.NewTimer(time.Second)
	nextPredictedStart := time.Now().Add(time.Second)
	name := ""

	for {

		select {
		case <-nextScheduledTimer.C:
			if name != "" {
				w.processWork(name)
			}
		case name = <-w.signal:
			w.processWork(name)
		}

		// drain timer
		select {
		case <-nextScheduledTimer.C:
		default:
		}

		var ok bool
		now := time.Now()

		nextPredictedStart, name, ok = w.getPredictedStart()
		if !ok {
			nextScheduledTimer.Reset(FallbackCheckInterval)
		} else {
			dur := nextPredictedStart.Sub(now)
			nextScheduledTimer.Reset(dur)
		}
	}
}

func (w *watcherInfo) getPredictedStart() (time.Time, string, bool) {
	info := &struct {
		QueueName  string    `db:"queue_name"`
		StartAfter time.Time `db:"start_after"`
	}{}
	ctx := context.Background()

	w.muListeningFor.RLock()
	defer w.muListeningFor.RUnlock()

	var validNames []string
	var busyNames []string
	for name, lInfo := range w.listeningFor {
		if !lInfo.isAtConcurrencyLimit() {
			validNames = append(validNames, name)
		} else {
			busyNames = append(busyNames, name)
		}
	}

	if len(validNames) == 0 {
		loggable := checkBusyLogGate(busyNames)
		if len(loggable) > 0 {
			Logger.Println("predicted start time: everyone is busy: checked=", strings.Join(loggable, ","))
		}

		return time.Time{}, "", false
	}

	err := model.WithTx(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		return model.GetContext(ctx, info, `
			select queue_name, min(start_after) "start_after"
			from pq_worker_queue 
			where started_at is null and queue_name = any ($1)
			group by queue_name
			order by min(start_after) asc
			limit 1
		`, modelutil.PqArray(validNames))
	})

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, "", false
		} else {
			Logger.Println("failed to get predicted start time", err)
			return time.Time{}, "", false
		}
	}

	return info.StartAfter.Local(), info.QueueName, true
}

func (w *watcherInfo) monitorDbNotifications() {
	for {
		watchForDbNotification(context.Background(), w.signal)

		// delay re-setting up connection b/c this is either a network or infrastructure issue
		<-time.After(1 * time.Second)
	}
}

func watchForDbNotification(ctx context.Context, signalC chan string) {
	conn, err := pqshared.Pool.Acquire(ctx)
	if err != nil {
		Logger.Println(err)
		return
	}

	defer conn.Release()

	_, err = conn.Exec(ctx, `listen pqworkerqueue`)
	if err != nil {
		Logger.Println(err)
		return
	}

	for {
		notif, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			Logger.Println(err)
			return
		}

		select {
		case signalC <- notif.Payload:
		default:
		}
	}
}

func cleaner() {
	er.HandleErrors(func(input *er.HandlerInput) {
		Logger.Println(input.Error, input.StackTrace)
	})

	tc := time.NewTicker(1 * time.Minute)

	for range tc.C {
		err := model.WithTx(context.Background(), func(ctx context.Context, tx *sqlx.Tx) error {
			return model.ExecContext(ctx, `
					delete from pq_worker_queue 
				    where id in (
				        select id
				        from pq_worker_queue
						where retain_until <= $1
						for update skip locked
				        limit 1000
				    )`, time.Now().UTC())
		})

		if err != nil {
			Logger.Println("failed to cleanup old records", err)
		}
	}
}

// Worker processes the job and can return a byte slice to be stored as a result
// Note: ctx is executed within a model.Tx
type Worker = func(ctx context.Context, id string, input json.RawMessage) []byte
type Middleware = func(next Worker) Worker

// NewWorkerGroup created a new group of workers to process the queue
func NewWorkerGroup(info *WorkerInfo) {
	StartWorkers(info)
}

func StartWorkers(info *WorkerInfo) {
	if info.NConcurrent == 0 {
		info.NConcurrent = 1
	}

	if info.RetainResultsFor == 0 {
		info.RetainResultsFor = 5 * time.Minute
	}

	if info.QueueName == "" {
		panic("missing QueueName")
	}

	if info.Callback == nil {
		panic("missing Callback")
	}

	addListen <- info
}

// DelayForLoadCallback if nil, no delay
type DelayForLoadCallback func(totalDelay time.Duration, delayCount int) *DelayInfo

type DelayInfo struct {
	DelayFor             time.Duration
	CheckAgainAfterDelay bool
}

type WorkerInfo struct {
	QueueName            string
	NConcurrent          int
	Callback             Worker
	Middleware           []Middleware
	RetainResultsFor     time.Duration
	TxIsolationLevel     *sql.IsolationLevel
	DelayForLoadCallback DelayForLoadCallback

	nActive   int
	muNActive sync.Mutex
}

func (w *WorkerInfo) isAtConcurrencyLimit() bool {
	w.muNActive.Lock()
	defer w.muNActive.Unlock()

	return w.nActive >= w.NConcurrent
}

func (w *WorkerInfo) concurrencyCheck() (cancel func(), ok bool) {
	w.muNActive.Lock()
	defer w.muNActive.Unlock()

	if w.nActive >= w.NConcurrent {
		return nil, false
	}

	w.nActive++

	cancel = func() {
		w.muNActive.Lock()
		w.nActive--
		w.muNActive.Unlock()
	}

	return cancel, true
}

type Status struct {
	ID          string
	Position    int
	CreatedAt   time.Time
	StartedAt   nulls.Time
	CompletedAt nulls.Time
}

func GetStatus(ctx context.Context, id string) (*Status, error) {

	queue := ""
	err := model.GetContext(ctx, &queue, `select queue_name from pq_worker_queue where id = $1`, id)
	if err != nil {
		return nil, err
	}

	status := &Status{}
	err = model.GetContext(ctx, status, `
		select 
			id,
			coalesce(rnk.position, 0) as "position",
			created_at, started_at, completed_at
		from pq_worker_queue r
		left join (
			select position
			from (
				select
					id,
					rank() over (order by created_at) as position
				from pq_worker_queue
				where queue_name = $1 and started_at is null
			) rk
			where rk.id = $2
		) rnk on 1=1
		where id = $2
	`, queue, id)

	return status, err
}

func GetResult(ctx context.Context, id string) ([]byte, error) {
	result := []byte{}
	err := model.QueryRowContext(ctx, `select result from pq_worker_queue where id = $1`, id).Scan(&result)
	return result, err
}

func NewQueue(name string) *Queue {
	return &Queue{name: name}
}

type Queue struct {
	name string
}

func (q *Queue) Name() string {
	return q.name
}

// Notify triggers the workers to process the new jobs available
func (q *Queue) Notify() error {
	_, err := pqshared.Pool.Exec(context.Background(), `select pg_notify('pqworkerqueue', $1)`, q.name)
	return err
}

// MustAdd pushes an item onto the queue and panics if there's a failure
// ctx must be called withing a model-transaction context
// arg must be json-encodable
// The item will be added using the model package, so it is transaction safe
func (q *Queue) MustAdd(ctx context.Context, arg interface{}) string {
	id, err := q.Add(ctx, arg)
	er.Check(err)

	return id
}

// Add pushes an item onto the queue.
// ctx must be called withing a model-transaction context
// arg must be json-encodable
// The item will be added using the model package, so it is transaction safe
func (q *Queue) Add(ctx context.Context, arg interface{}) (string, error) {
	return q.add(ctx, time.Now().UTC(), "", true, nil, arg)
}

func (q *Queue) MustAddOpt(ctx context.Context, arg interface{}, opts *AddOption) string {
	id, err := q.AddOpt(ctx, arg, opts)
	er.Check(err)

	return id
}

type AddOption struct {
	// StartAfter schedules the job to start after the given time
	StartAfter time.Time

	// DebounceKey combined with QueueName are used to skip duplicate jobs.
	// DebounceKey does not guarantee that a job won't be run twice, but ensures it doesn't happen during the normal
	// course of things. If a job is run twice, it will be because the first job was in progress or being updated by
	// another transaction when the second job was added.
	//
	// if DebounceKey is empty, no debouncing will be done
	DebounceKey string

	// DebounceKeepOriginalStart if true, will keep the original startAfter if another (QueueName,DebounceKey) exists
	// if false, the new deadline will replace the old one
	DebounceKeepOriginalStart bool

	DebounceMerge func(a []byte, b []byte) ([]byte, error)
}

func (q *Queue) AddOpt(ctx context.Context, arg interface{}, opts *AddOption) (string, error) {
	return q.add(ctx, opts.StartAfter, opts.DebounceKey, opts.DebounceKeepOriginalStart, opts.DebounceMerge, arg)
}

func (q *Queue) add(ctx context.Context, startAfter time.Time, debounceKey string, keepOriginalStart bool, debounceMerge func(a []byte, b []byte) ([]byte, error), arg interface{}) (string, error) {
	msg, err := json.Marshal(arg)
	if err != nil {
		return "", errors.Wrap(err, "failed to json-encode work-queue arg")
	}

	uuidId, err := uuid.NewRandom()
	if err != nil {
		return "", errors.Wrap(err, "failed to assign id to job")
	}

	model.OnTransactionCommitted(ctx, func() {
		if err := q.Notify(); err != nil {
			Logger.Println("failed to notify:", err)
		}
	})

	if startAfter.IsZero() {
		startAfter = time.Now().UTC()
	} else {
		startAfter = startAfter.UTC()
	}

	if debounceKey == "" {
		err = model.ExecContext(ctx, `
			insert into pq_worker_queue (id, queue_name, job_arg, created_at, start_after, debounce_key) 
			values ($1, $2, $3, $4, $5, $6)
		`,
			uuidId.String(), q.name, msg, time.Now().UTC(),
			startAfter, debounceKey,
		)

		return uuidId.String(), err
	}

	if debounceMerge != nil {
		if debounceKey == "" {
			return "", errors.New("debounceMerge requires a debounceKey")
		}

		obj := struct {
			ID         string    `db:"id"`
			JobArg     []byte    `db:"job_arg"`
			StartAfter time.Time `db:"start_after"`
		}{}

		err = model.GetContext(ctx, &obj, `
			with existing as (
			    select id, job_arg, start_after
			    from pq_worker_queue
			    where queue_name = $2 and debounce_key = $6 and started_at is null
			    limit 1
			    for update skip locked
			), new as (
			    insert into pq_worker_queue (id, queue_name, job_arg, created_at, start_after, debounce_key) 
				select $1, $2, $3, $4, $5, $6
				where (select count(*) from existing) = 0
			)
			select id, job_arg, start_after
			from existing
		`,
			uuidId.String(), q.name, msg, time.Now().UTC(),
			startAfter, debounceKey,
		)

		if err != nil {
			// created new job
			if errors.Is(err, sql.ErrNoRows) {
				return uuidId.String(), nil
			}

			return "", err
		}

		merged, err2 := debounceMerge(obj.JobArg, msg)
		if err2 != nil {
			return "", err2
		}

		if !keepOriginalStart {
			obj.StartAfter = startAfter
		}

		err = model.ExecContext(ctx, `
			update pq_worker_queue 
			set job_arg = $1, start_after = $2 
			where id = $3`,
			merged, obj.StartAfter, obj.ID)

		return obj.ID, err
	}

	// resultID is the id of the row that actually represents this job afterwards: either the
	// pre-existing debounced row or the newly-inserted one. We must return that (not uuidId
	// unconditionally) so callers can poll status/result by the returned id.
	resultID := ""

	if keepOriginalStart {
		err = model.GetContext(ctx, &resultID, `
			with existing as (
			    select id
			    from pq_worker_queue
			    where queue_name = $2 and debounce_key = $6 and $6 != '' and started_at is null
			    limit 1
			    for update skip locked
			), new as (
			    insert into pq_worker_queue (id, queue_name, job_arg, created_at, start_after, debounce_key)
				select $1, $2, $3, $4, $5, $6
				where (select count(*) from existing) = 0
				returning id
			)
			select id from existing
			union all
			select id from new
		`,
			uuidId.String(), q.name, msg, time.Now().UTC(),
			startAfter, debounceKey,
		)
	} else {
		err = model.GetContext(ctx, &resultID, `
			with existing as (
			    select id
			    from pq_worker_queue
			    where queue_name = $2 and debounce_key = $6 and $6 != '' and started_at is null
			    limit 1
			    for update skip locked
			), new as (
			    insert into pq_worker_queue (id, queue_name, job_arg, created_at, start_after, debounce_key)
				select $1, $2, $3, $4, $5, $6
				where (select count(*) from existing) = 0
				returning id
			), upd as (
			    update pq_worker_queue
			    set start_after = $5
				from existing
				where existing.id = pq_worker_queue.id
				returning pq_worker_queue.id
			)
			select id from existing
			union all
			select id from new
		`,
			uuidId.String(), q.name, msg, time.Now().UTC(),
			startAfter, debounceKey,
		)
	}

	return resultID, err
}
