package mysql

import (
	"context"
	"database/sql"
	"errors"
	"math/rand"
	"regexp"
	"strings"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var ErrSlaveExec = errors.New("exec cmd on slave node is forbidden")
var sqlLockCheckReg = regexp.MustCompile(`(?i)\bFOR\s+(UPDATE|SHARE|KEY\s+SHARE)\b`)

type cdb struct {
	db     *sql.DB
	master bool
	addr   string
	name   string
	tracer trace.Tracer
}

func (c *cdb) stats() sql.DBStats {
	return c.db.Stats()
}
func (c *cdb) pingContext(ctx context.Context) error {
	_, span := c.tracer.Start(ctx, "call mysql", trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("sname", c.name),
			attribute.String("sip", c.addr),
			attribute.String("mysql.cmd", "Ping"),
		),
	)
	defer span.End()
	if c.master {
		span.SetAttributes(attribute.String("mysql.role", "master"))
	} else {
		span.SetAttributes(attribute.String("mysql.role", "slave"))
	}
	e := c.db.PingContext(ctx)
	if e != nil {
		span.SetStatus(codes.Error, e.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	return e
}

func (c *cdb) queryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	_, span := c.tracer.Start(ctx, "call mysql", trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("sname", c.name),
			attribute.String("sip", c.addr),
			attribute.String("mysql.cmd", "QueryRow"),
			attribute.String("mysql.sql", query),
		),
	)
	defer span.End()
	if c.master {
		span.SetAttributes(attribute.String("mysql.role", "master"))
	} else {
		span.SetAttributes(attribute.String("mysql.role", "slave"))
	}
	r := c.db.QueryRowContext(ctx, query, args...)
	if r.Err() != nil {
		span.SetStatus(codes.Error, r.Err().Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	return r
}
func (c *cdb) queryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	_, span := c.tracer.Start(ctx, "call mysql", trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("sname", c.name),
			attribute.String("sip", c.addr),
			attribute.String("mysql.cmd", "Query"),
			attribute.String("mysql.sql", query),
		),
	)
	defer span.End()
	if c.master {
		span.SetAttributes(attribute.String("mysql.role", "master"))
	} else {
		span.SetAttributes(attribute.String("mysql.role", "slave"))
	}
	rs, e := c.db.QueryContext(ctx, query, args...)
	if e != nil {
		span.SetStatus(codes.Error, e.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	return rs, e
}
func (c *cdb) execContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if !c.master {
		return nil, ErrSlaveExec
	}
	_, span := c.tracer.Start(ctx, "call mysql", trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("sname", c.name),
			attribute.String("sip", c.addr),
			attribute.String("mysql.cmd", "Exec"),
			attribute.String("mysql.sql", query),
		),
	)
	defer span.End()
	if c.master {
		span.SetAttributes(attribute.String("mysql.role", "master"))
	} else {
		span.SetAttributes(attribute.String("mysql.role", "slave"))
	}
	r, e := c.db.ExecContext(ctx, query, args...)
	if e != nil {
		span.SetStatus(codes.Error, e.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	return r, e
}
func (c *cdb) beginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	_, span := c.tracer.Start(ctx, "call mysql", trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("sname", c.name),
			attribute.String("sip", c.addr),
			attribute.String("mysql.cmd", "Begin"),
		),
	)
	defer span.End()
	if c.master {
		span.SetAttributes(attribute.String("mysql.role", "master"))
	} else {
		span.SetAttributes(attribute.String("mysql.role", "slave"))
	}
	tx, e := c.db.BeginTx(ctx, opts)
	if e != nil {
		span.SetStatus(codes.Error, e.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	return tx, e
}
func (c *cdb) prepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	if !c.master {
		tmpquery := strings.TrimSpace(query)
		if len(tmpquery) < 6 || !strings.EqualFold(tmpquery[:6], "SELECT") {
			return nil, ErrSlaveExec
		}
		if sqlLockCheckReg.MatchString(tmpquery) {
			return nil, ErrSlaveExec
		}
	}
	_, span := c.tracer.Start(ctx, "call mysql", trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("sname", c.name),
			attribute.String("sip", c.addr),
			attribute.String("mysql.cmd", "Prepare"),
			attribute.String("mysql.sql", query),
		),
	)
	defer span.End()
	if c.master {
		span.SetAttributes(attribute.String("mysql.role", "master"))
	} else {
		span.SetAttributes(attribute.String("mysql.role", "slave"))
	}
	stmt, e := c.db.PrepareContext(ctx, query)
	if e != nil {
		span.SetStatus(codes.Error, e.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	return stmt, e
}
func (c *cdb) close() error {
	return c.db.Close()
}

type Operator []*cdb

func (o Operator) Stats() map[string]sql.DBStats {
	r := make(map[string]sql.DBStats)
	for _, db := range o {
		r[db.addr] = db.stats()
	}
	return r
}
func (o Operator) PingContext(ctx context.Context) error {
	//len(o) == 0  can't be happened,Client.Master or Client.Slave will panic if len() is 0
	if len(o) == 1 {
		return o[0].pingContext(ctx)
	}
	lker := sync.Mutex{}
	var e error
	wg := &sync.WaitGroup{}
	for _, v := range o {
		db := v
		wg.Go(func() {
			if err := db.pingContext(ctx); err != nil {
				lker.Lock()
				e = err
				lker.Unlock()
			}
		})
	}
	wg.Wait()
	return e
}
func (o Operator) Close() error {
	//len(o) == 0  can't be happened,Client.Master or Client.Slave will panic if len() is 0
	if len(o) == 1 {
		return o[0].close()
	}
	lker := sync.Mutex{}
	var e error
	wg := &sync.WaitGroup{}
	for _, v := range o {
		db := v
		wg.Go(func() {
			if err := db.close(); err != nil {
				lker.Lock()
				e = err
				lker.Unlock()
			}
		})
	}
	wg.Wait()
	return e
}

func (o Operator) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	//len(o) == 0  can't be happened,Client.Master or Client.Slave will panic if len() is 0
	if len(o) == 1 {
		return o[0].queryRowContext(ctx, query, args...)
	}
	return o[rand.Intn(len(o))].queryRowContext(ctx, query, args...)
}

func (o Operator) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	//len(o) == 0  can't be happened,Client.Master or Client.Slave will panic if len() is 0
	if len(o) == 1 {
		return o[0].queryContext(ctx, query, args...)
	}
	return o[rand.Intn(len(o))].queryContext(ctx, query, args...)
}

func (o Operator) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	//len(o) == 0  can't be happened,Client.Master or Client.Slave will panic if len() is 0
	if len(o) == 1 {
		if !o[0].master {
			return nil, ErrSlaveExec
		}
		return o[0].execContext(ctx, query, args...)
	}
	db := o[rand.Intn(len(o))]
	if !db.master {
		return nil, ErrSlaveExec
	}
	return db.execContext(ctx, query, args...)
}

type Tx struct {
	t  *sql.Tx
	db *cdb
}

func (t *Tx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	_, span := t.db.tracer.Start(ctx, "call mysql", trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("sname", t.db.name),
			attribute.String("sip", t.db.addr),
			attribute.String("mysql.cmd", "TxQueryRow"),
			attribute.String("mysql.sql", query),
		),
	)
	defer span.End()
	if t.db.master {
		span.SetAttributes(attribute.String("mysql.role", "master"))
	} else {
		span.SetAttributes(attribute.String("mysql.role", "slave"))
	}
	r := t.t.QueryRowContext(ctx, query, args...)
	if r.Err() != nil {
		span.SetStatus(codes.Error, r.Err().Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	return r
}
func (t *Tx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	_, span := t.db.tracer.Start(ctx, "call mysql", trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("sname", t.db.name),
			attribute.String("sip", t.db.addr),
			attribute.String("mysql.cmd", "TxQuery"),
			attribute.String("mysql.sql", query),
		),
	)
	defer span.End()
	if t.db.master {
		span.SetAttributes(attribute.String("mysql.role", "master"))
	} else {
		span.SetAttributes(attribute.String("mysql.role", "slave"))
	}
	rs, e := t.t.QueryContext(ctx, query, args...)
	if e != nil {
		span.SetStatus(codes.Error, e.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	return rs, e
}
func (t *Tx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if !t.db.master {
		return nil, ErrSlaveExec
	}
	_, span := t.db.tracer.Start(ctx, "call mysql", trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("sname", t.db.name),
			attribute.String("sip", t.db.addr),
			attribute.String("mysql.cmd", "TxExec"),
			attribute.String("mysql.sql", query),
		),
	)
	defer span.End()
	if t.db.master {
		span.SetAttributes(attribute.String("mysql.role", "master"))
	} else {
		span.SetAttributes(attribute.String("mysql.role", "slave"))
	}
	r, e := t.t.ExecContext(ctx, query, args...)
	if e != nil {
		span.SetStatus(codes.Error, e.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	return r, e
}

// Warning!Don't forget to close the stmt!
// Stmt should be reused in this trasnaction until it is not needed anymore in this trasnaction
// it will occupy a connection until it be closed
func (t *Tx) PrepareContext(ctx context.Context, query string) (*Stmt, error) {
	if !t.db.master {
		tmpquery := strings.TrimSpace(query)
		if len(tmpquery) < 6 || !strings.EqualFold(tmpquery[:6], "SELECT") {
			return nil, ErrSlaveExec
		}
		if sqlLockCheckReg.MatchString(tmpquery) {
			return nil, ErrSlaveExec
		}
	}
	_, span := t.db.tracer.Start(ctx, "call mysql", trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("sname", t.db.name),
			attribute.String("sip", t.db.addr),
			attribute.String("mysql.cmd", "TxPrepare"),
			attribute.String("mysql.sql", query),
		),
	)
	defer span.End()
	if t.db.master {
		span.SetAttributes(attribute.String("mysql.role", "master"))
	} else {
		span.SetAttributes(attribute.String("mysql.role", "slave"))
	}
	newstmt, e := t.t.PrepareContext(ctx, query)
	if e != nil {
		span.SetStatus(codes.Error, e.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	return &Stmt{
		tx:    t,
		query: query,
		stmts: map[*cdb]*sql.Stmt{t.db: newstmt},
	}, nil
}
func (t *Tx) Commit(ctx context.Context) error {
	_, span := t.db.tracer.Start(ctx, "call mysql", trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("sname", t.db.name),
			attribute.String("sip", t.db.addr),
			attribute.String("mysql.cmd", "Commit"),
		),
	)
	defer span.End()
	if t.db.master {
		span.SetAttributes(attribute.String("mysql.role", "master"))
	} else {
		span.SetAttributes(attribute.String("mysql.role", "slave"))
	}
	e := t.t.Commit()
	if e != nil {
		span.SetStatus(codes.Error, e.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	return e
}
func (t *Tx) Rollback(ctx context.Context) error {
	_, span := t.db.tracer.Start(ctx, "call mysql", trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("sname", t.db.name),
			attribute.String("sip", t.db.addr),
			attribute.String("mysql.cmd", "Rollback"),
		),
	)
	defer span.End()
	if t.db.master {
		span.SetAttributes(attribute.String("mysql.role", "master"))
	} else {
		span.SetAttributes(attribute.String("mysql.role", "slave"))
	}
	e := t.t.Rollback()
	if e != nil {
		span.SetStatus(codes.Error, e.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	return e
}
func (o Operator) BeginTx(ctx context.Context, opts *sql.TxOptions) (*Tx, error) {
	//len(o) == 0  can't be happened,Client.Master or Client.Slave will panic if len() is 0
	if len(o) == 1 {
		tx, e := o[0].beginTx(ctx, opts)
		return &Tx{
			t:  tx,
			db: o[0],
		}, e
	}
	db := o[rand.Intn(len(o))]
	tx, e := db.beginTx(ctx, opts)
	return &Tx{
		t:  tx,
		db: db,
	}, e
}

type Stmt struct {
	tx    *Tx
	query string
	stmts map[*cdb]*sql.Stmt
}

func (s *Stmt) QueryRowContext(ctx context.Context, args ...any) *sql.Row {
	//len(o) == 0  can't be happened,Client.Master or Client.Slave will panic if len() is 0
	var stmt *sql.Stmt
	var db *cdb
	for db, stmt = range s.stmts {
		//this is random
		break
	}
	_, span := db.tracer.Start(ctx, "call mysql", trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("sname", db.name),
			attribute.String("sip", db.addr),
		),
	)
	defer span.End()
	if s.tx == nil {
		span.SetAttributes(attribute.String("mysql.cmd", "StmtQueryRow"))
	} else {
		span.SetAttributes(attribute.String("mysql.cmd", "TxStmtQueryRow"))
	}
	span.SetAttributes(attribute.String("mysql.sql", s.query))
	if db.master {
		span.SetAttributes(attribute.String("mysql.role", "master"))
	} else {
		span.SetAttributes(attribute.String("mysql.role", "slave"))
	}
	r := stmt.QueryRowContext(ctx, args...)
	if r.Err() != nil {
		span.SetStatus(codes.Error, r.Err().Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	return r
}
func (s *Stmt) QueryContext(ctx context.Context, args ...any) (*sql.Rows, error) {
	//len(o) == 0  can't be happened,Client.Master or Client.Slave will panic if len() is 0
	var stmt *sql.Stmt
	var db *cdb
	for db, stmt = range s.stmts {
		//this is random
		break
	}
	_, span := db.tracer.Start(ctx, "call mysql", trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("sname", db.name),
			attribute.String("sip", db.addr),
		),
	)
	defer span.End()
	if s.tx == nil {
		span.SetAttributes(attribute.String("mysql.cmd", "StmtQuery"))
	} else {
		span.SetAttributes(attribute.String("mysql.cmd", "TxStmtQuery"))
	}
	span.SetAttributes(attribute.String("mysql.sql", s.query))
	if db.master {
		span.SetAttributes(attribute.String("mysql.role", "master"))
	} else {
		span.SetAttributes(attribute.String("mysql.role", "slave"))
	}
	rs, e := stmt.QueryContext(ctx, args...)
	if e != nil {
		span.SetStatus(codes.Error, e.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	return rs, e
}
func (s *Stmt) ExecContext(ctx context.Context, args ...any) (sql.Result, error) {
	//len(o) == 0  can't be happened,Client.Master or Client.Slave will panic if len() is 0
	var stmt *sql.Stmt
	var db *cdb
	for db, stmt = range s.stmts {
		//this is random
		break
	}
	if !db.master {
		return nil, ErrSlaveExec
	}
	_, span := db.tracer.Start(ctx, "call mysql", trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("sname", db.name),
			attribute.String("sip", db.addr),
		),
	)
	defer span.End()
	if s.tx == nil {
		span.SetAttributes(attribute.String("mysql.cmd", "StmtExec"))
	} else {
		span.SetAttributes(attribute.String("mysql.cmd", "TxStmtExec"))
	}
	span.SetAttributes(attribute.String("mysql.sql", s.query))
	if db.master {
		span.SetAttributes(attribute.String("mysql.role", "master"))
	} else {
		span.SetAttributes(attribute.String("mysql.role", "slave"))
	}
	r, e := stmt.ExecContext(ctx, args...)
	if e != nil {
		span.SetStatus(codes.Error, e.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	return r, e
}
func (s *Stmt) Close() error {
	//len(o) == 0  can't be happened,Client.Master or Client.Slave will panic if len() is 0
	if len(s.stmts) == 1 {
		var stmt *sql.Stmt
		for _, stmt = range s.stmts {
			break
		}
		return stmt.Close()
	}
	lker := sync.Mutex{}
	var e error
	wg := &sync.WaitGroup{}
	for _, v := range s.stmts {
		stmt := v
		wg.Go(func() {
			if err := stmt.Close(); err != nil {
				lker.Lock()
				e = err
				lker.Unlock()
			}
		})
	}
	wg.Wait()
	return e
}

// Warning!Don't forget to close the stmt!
// Stmt should be reused until it is not needed anymore
// it will occupy a connection until it be closed
func (o Operator) PrepareContext(ctx context.Context, query string) (*Stmt, error) {
	//len(o) == 0  can't be happened,Client.Master or Client.Slave will panic if len() is 0
	if len(o) == 1 {
		stmt, e := o[0].prepareContext(ctx, query)
		if e != nil {
			return nil, e
		}
		return &Stmt{
			query: query,
			stmts: map[*cdb]*sql.Stmt{
				o[0]: stmt,
			},
		}, nil
	}
	lker := sync.Mutex{}
	var e error
	stmts := make(map[*cdb]*sql.Stmt)
	wg := &sync.WaitGroup{}
	for _, v := range o {
		db := v
		wg.Go(func() {
			stmt, err := db.prepareContext(ctx, query)
			lker.Lock()
			defer lker.Unlock()
			if err != nil {
				e = err
				return
			}
			stmts[db] = stmt
		})
	}
	wg.Wait()
	if e != nil {
		for _, stmt := range stmts {
			if stmt == nil {
				continue
			}
			go stmt.Close()
		}
		return nil, e
	}
	return &Stmt{query: query, stmts: stmts}, nil
}
