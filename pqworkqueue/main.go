package pqworkqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"

	"github.com/gobuffalo/nulls"
	"github.com/gofrs/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/ntbosscher/gobase/env"
	"github.com/ntbosscher/gobase/er"
	"github.com/ntbosscher/gobase/model"
	"github.com/ntbosscher/gobase/pqshared"
	"github.com/pkg/errors"
)

var addListen chan *WorkerInfo

func init() {

	addListen = make(chan *WorkerInfo)
	var err error

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

		_, err = pqshared.Pool.Exec(context.Background(), `create index if not exists ix_pq_worker_queue_pending on pq_worker_queue (queue_name, start_after) where started_at is not null;`)
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
	}

	signalTimers := make(chan bool)

	go w.addNewListeners()
	go w.checkWorkBasedOnTimers(signalTimers)

	for {
		waitForNotification(context.Background(), w.processWork, signalTimers)

		// delay re-setting up connection b/c this is either a network or infrastructure issue
		<-time.After(1 * time.Second)
	}
}

func (w *watcherInfo) processWork(queueName string) {
	defer er.HandleErrors(func(input *er.HandlerInput) {
		Logger.Println(input.Error, input.StackTrace)
	})

	mightBeMore := true

	for mightBeMore {
		mightBeMore = w.startWork(queueName)
	}
}

func (w *watcherInfo) startWorkConcurrencyCheck(queueName string) (ok bool, isolationLevel sql.IsolationLevel, callback Worker, middleware []Middleware, retainResultsFor time.Duration, done func()) {
	w.muListeningFor.RLock()
	defer w.muListeningFor.RUnlock()

	info := w.listeningFor[queueName]
	if info == nil {
		Logger.Println("missing info for queue name", queueName)
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
	return
}

func (w *watcherInfo) startWork(queueName string) (mightBeMore bool) {

	ok, isolationLevel, callback, middleware, retainResultsFor, cancel := w.startWorkConcurrencyCheck(queueName)
	if !ok {
		return false
	}

	defer cancel()

	err := model.WithTx(context.Background(), func(ctx context.Context, tx *sqlx.Tx) error {
		id, message, err := getAndClaimJob(ctx, queueName)
		if err != nil {
			if err != sql.ErrNoRows {
				Logger.Println("failed to get job:", err.Error())
			}

			return err
		}

		var result []byte

		err2 := model.WithTx2(context.Background(), isolationLevel, func(ctx context.Context, tx *sqlx.Tx) error {
			exec := callback
			for _, item := range middleware {
				exec = item(exec)
			}

			result = exec(ctx, id, message)
			return nil
		})

		commitErr := nulls.String{}
		if err2 != nil {
			Logger.Println("failed to process job", err2)
			commitErr = nulls.NewString(err2.Error())
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

	if err == sql.ErrNoRows {
		return false
	}

	if err != nil {
		Logger.Println("failed to commit transaction", err.Error())
		return true
	}

	return true
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

		go w.processWork(info.QueueName)
	}
}

func (w *watcherInfo) checkWorkBasedOnTimers(checkC chan bool) {
	defer er.HandleErrors(func(input *er.HandlerInput) {
		Logger.Println(input.Error, input.StackTrace)
	})

	timer := time.NewTimer(time.Second)
	nextPredictedStart := time.Now().Add(time.Second)
	name := ""

	for {
		timer.Reset(nextPredictedStart.Sub(time.Now()))

		select {
		case <-checkC:
		case <-timer.C:
			if name != "" {
				w.processWork(name)
			}
		}

		var ok bool
		nextPredictedStart, name, ok = w.getPredictedStart()
		if !ok {
			nextPredictedStart = time.Now().Add(time.Hour)
		}
	}
}

func (w *watcherInfo) getPredictedStart() (time.Time, string, bool) {
	info := &struct {
		QueueName  string    `db:"queue_name"`
		StartAfter time.Time `db:"start_after"`
	}{}
	ctx := context.Background()

	err := model.WithTx(ctx, func(ctx context.Context, tx *sqlx.Tx) error {
		return model.GetContext(ctx, info, `
			select queue_name, min(start_after) "start_after"
			from pq_worker_queue 
			where started_at is null
			group by queue_name
			order by min(start_after) asc
			limit 1
		`)
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

var Logger = log.New(os.Stderr, "pqworkerqueue", log.Llongfile)

func waitForNotification(ctx context.Context, callback func(queueName string), signalC chan bool) {
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

		callback(notif.Payload)

		select {
		case signalC <- true:
		default:
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

type WorkerInfo struct {
	QueueName        string
	NConcurrent      int
	Callback         Worker
	Middleware       []Middleware
	RetainResultsFor time.Duration
	TxIsolationLevel *sql.IsolationLevel

	nActive   int
	muNActive sync.Mutex
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

	uuidId, err := uuid.NewV4()
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

	if keepOriginalStart {
		err = model.ExecContext(ctx, `
			with existing as (
			    select id, job_arg, start_after
			    from pq_worker_queue
			    where queue_name = $2 and debounce_key = $6 and $6 != '' and started_at is null
			    limit 1
			    for update skip locked
			)
			insert into pq_worker_queue (id, queue_name, job_arg, created_at, start_after, debounce_key) 
			select $1, $2, $3, $4, $5, $6
			where (select count(*) from existing) = 0
		`,
			uuidId.String(), q.name, msg, time.Now().UTC(),
			startAfter, debounceKey,
		)
	} else {
		err = model.ExecContext(ctx, `
			with existing as (
			    select id, job_arg, start_after
			    from pq_worker_queue
			    where queue_name = $2 and debounce_key = $6 and $6 != '' and started_at is null
			    limit 1
			    for update skip locked
			), new as (
			    insert into pq_worker_queue (id, queue_name, job_arg, created_at, start_after, debounce_key) 
				select $1, $2, $3, $4, $5, $6
				where (select count(*) from existing) = 0
			)
			update pq_worker_queue 
		    set start_after = $5
			from existing
			where existing.id = pq_worker_queue.id
		`,
			uuidId.String(), q.name, msg, time.Now().UTC(),
			startAfter, debounceKey,
		)
	}

	return uuidId.String(), err
}
