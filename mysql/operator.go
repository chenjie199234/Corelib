package mysql

import (
	"context"
	"database/sql"
	"errors"
	"math/rand"
	"strings"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var ErrSlaveExec = errors.New("do exec cmd on slave node")

type cdb struct {
	db     *sql.DB
	master bool
	addr   string
	name   string
}

func (c *cdb) Stats() sql.DBStats {
	return c.db.Stats()
}
func (c *cdb) PingContext(ctx context.Context) error {
	_, span := otel.Tracer("").Start(
		ctx,
		"call mysql",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("server.name", c.name),
			attribute.String("server.addr", c.addr),
			attribute.String("mysql.cmd", "Ping"),
		),
	)
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
	span.End()
	return e
}
func (c *cdb) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	_, span := otel.Tracer("").Start(
		ctx,
		"call mysql",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("server.name", c.name),
			attribute.String("server.addr", c.addr),
			attribute.String("mysql.cmd", "QueryRow"),
		),
	)
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
	span.End()
	return r
}
func (c *cdb) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	_, span := otel.Tracer("").Start(
		ctx,
		"call mysql",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("server.name", c.name),
			attribute.String("server.addr", c.addr),
			attribute.String("mysql.cmd", "Query"),
		),
	)
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
	span.End()
	return rs, e
}
func (c *cdb) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if !c.master {
		return nil, ErrSlaveExec
	}
	_, span := otel.Tracer("").Start(
		ctx,
		"call mysql",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("server.name", c.name),
			attribute.String("server.addr", c.addr),
			attribute.String("mysql.cmd", "Exec"),
		),
	)
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
	span.End()
	return r, e
}
func (c *cdb) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	_, span := otel.Tracer("").Start(
		ctx,
		"call mysql",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("server.name", c.name),
			attribute.String("server.addr", c.addr),
			attribute.String("mysql.cmd", "Begin"),
		),
	)
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
	span.End()
	return tx, e
}
func (c *cdb) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	if !c.master {
		tmpquery := strings.TrimSpace(query)
		if tmpquery[0] != 'S' && tmpquery[0] != 's' {
			return nil, ErrSlaveExec
		}
		if tmpquery[1] != 'E' && tmpquery[1] != 'e' {
			return nil, ErrSlaveExec
		}
		if tmpquery[2] != 'L' && tmpquery[2] != 'l' {
			return nil, ErrSlaveExec
		}
		if tmpquery[3] != 'E' && tmpquery[3] != 'e' {
			return nil, ErrSlaveExec
		}
		if tmpquery[4] != 'C' && tmpquery[4] != 'c' {
			return nil, ErrSlaveExec
		}
		if tmpquery[5] != 'T' && tmpquery[5] != 't' {
			return nil, ErrSlaveExec
		}
	}
	_, span := otel.Tracer("").Start(
		ctx,
		"call mysql",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("server.name", c.name),
			attribute.String("server.addr", c.addr),
			attribute.String("mysql.cmd", "Prepare"),
		),
	)
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
	span.End()
	return stmt, e
}
func (c *cdb) Close() error {
	return c.db.Close()
}

type Operator []*cdb

func (o Operator) Stats() map[string]sql.DBStats {
	r := make(map[string]sql.DBStats)
	for _, db := range o {
		r[db.addr] = db.Stats()
	}
	return r
}
func (o Operator) PingContext(ctx context.Context) error {
	if len(o) == 0 {
		return nil
	} else if len(o) == 1 {
		return o[0].PingContext(ctx)
	} else {
		var e error
		wg := &sync.WaitGroup{}
		for _, v := range o {
			db := v
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := db.PingContext(ctx); err != nil {
					e = err
				}
			}()
		}
		wg.Wait()
		return e
	}
}
func (o Operator) Close() error {
	if len(o) == 0 {
		return nil
	} else if len(o) == 1 {
		return o[0].Close()
	} else {
		var e error
		wg := &sync.WaitGroup{}
		for _, v := range o {
			db := v
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := db.Close(); err != nil {
					e = err
				}
			}()
		}
		wg.Wait()
		return e
	}
}

func (o Operator) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if len(o) == 1 {
		return o[0].QueryRowContext(ctx, query, args...)
	}
	return o[rand.Intn(len(o))].QueryRowContext(ctx, query, args...)
}

func (o Operator) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if len(o) == 1 {
		return o[0].QueryContext(ctx, query, args...)
	}
	return o[rand.Intn(len(o))].QueryContext(ctx, query, args...)
}

func (o Operator) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if len(o) == 1 {
		return o[0].ExecContext(ctx, query, args...)
	}
	return o[rand.Intn(len(o))].ExecContext(ctx, query, args...)
}

type Tx struct {
	t  *sql.Tx
	db *cdb
}

func (t *Tx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	_, span := otel.Tracer("").Start(
		ctx,
		"call mysql",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("server.name", t.db.name),
			attribute.String("server.addr", t.db.addr),
			attribute.String("mysql.cmd", "TxQueryRow"),
		),
	)
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
	span.End()
	return r
}
func (t *Tx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	_, span := otel.Tracer("").Start(
		ctx,
		"call mysql",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("server.name", t.db.name),
			attribute.String("server.addr", t.db.addr),
			attribute.String("mysql.cmd", "TxQuery"),
		),
	)
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
	span.End()
	return rs, e
}
func (t *Tx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if !t.db.master {
		return nil, ErrSlaveExec
	}
	_, span := otel.Tracer("").Start(
		ctx,
		"call mysql",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("server.name", t.db.name),
			attribute.String("server.addr", t.db.addr),
			attribute.String("mysql.cmd", "TxExec"),
		),
	)
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
	span.End()
	return r, e
}

// the returned stmt don't need to close manually,it will be closed by commit or rollback
func (t *Tx) PrepareContext(ctx context.Context, query string) (*Stmt, error) {
	if !t.db.master {
		tmpquery := strings.TrimSpace(query)
		if tmpquery[0] != 'S' && tmpquery[0] != 's' {
			return nil, ErrSlaveExec
		}
		if tmpquery[1] != 'E' && tmpquery[1] != 'e' {
			return nil, ErrSlaveExec
		}
		if tmpquery[2] != 'L' && tmpquery[2] != 'l' {
			return nil, ErrSlaveExec
		}
		if tmpquery[3] != 'E' && tmpquery[3] != 'e' {
			return nil, ErrSlaveExec
		}
		if tmpquery[4] != 'C' && tmpquery[4] != 'c' {
			return nil, ErrSlaveExec
		}
		if tmpquery[5] != 'T' && tmpquery[5] != 't' {
			return nil, ErrSlaveExec
		}
	}
	_, span := otel.Tracer("").Start(
		ctx,
		"call mysql",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("server.name", t.db.name),
			attribute.String("server.addr", t.db.addr),
			attribute.String("mysql.cmd", "TxPrepare"),
		),
	)
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
	span.End()
	return &Stmt{
		tx:    t,
		query: query,
		stmts: map[*cdb]*sql.Stmt{t.db: newstmt},
	}, nil
}
func (t *Tx) Commit(ctx context.Context) error {
	_, span := otel.Tracer("").Start(
		ctx,
		"call mysql",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("server.name", t.db.name),
			attribute.String("server.addr", t.db.addr),
			attribute.String("mysql.cmd", "Commit"),
		),
	)
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
	span.End()
	return e
}
func (t *Tx) Rollback(ctx context.Context) error {
	_, span := otel.Tracer("").Start(
		ctx,
		"call mysql",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("server.name", t.db.name),
			attribute.String("server.addr", t.db.addr),
			attribute.String("mysql.cmd", "Rollback"),
		),
	)
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
	span.End()
	return e
}
func (o Operator) BeginTx(ctx context.Context, opts *sql.TxOptions) (*Tx, error) {
	if len(o) == 1 {
		tx, e := o[0].BeginTx(ctx, opts)
		return &Tx{
			t:  tx,
			db: o[0],
		}, e
	}
	db := o[rand.Intn(len(o))]
	tx, e := db.BeginTx(ctx, opts)
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
	var stmt *sql.Stmt
	var db *cdb
	for db, stmt = range s.stmts {
		//this is random
		break
	}
	_, span := otel.Tracer("").Start(
		ctx,
		"call mysql",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("server.name", db.name),
			attribute.String("server.addr", db.addr),
		),
	)
	if s.tx == nil {
		span.SetAttributes(attribute.String("mysql.cmd", "StmtQueryRow"))
	} else {
		span.SetAttributes(attribute.String("mysql.cmd", "TxStmtQueryRow"))
	}
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
	span.End()
	return r
}
func (s *Stmt) QueryContext(ctx context.Context, args ...any) (*sql.Rows, error) {
	var stmt *sql.Stmt
	var db *cdb
	for db, stmt = range s.stmts {
		//this is random
		break
	}
	_, span := otel.Tracer("").Start(
		ctx,
		"call mysql",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("server.name", db.name),
			attribute.String("server.addr", db.addr),
		),
	)
	if s.tx == nil {
		span.SetAttributes(attribute.String("mysql.cmd", "StmtQuery"))
	} else {
		span.SetAttributes(attribute.String("mysql.cmd", "TxStmtQuery"))
	}
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
	span.End()
	return rs, e
}
func (s *Stmt) ExecContext(ctx context.Context, args ...any) (sql.Result, error) {
	var stmt *sql.Stmt
	var db *cdb
	for db, stmt = range s.stmts {
		//this is random
		break
	}
	if !db.master {
		return nil, ErrSlaveExec
	}
	_, span := otel.Tracer("").Start(
		ctx,
		"call mysql",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("server.name", db.name),
			attribute.String("server.addr", db.addr),
		),
	)
	if s.tx == nil {
		span.SetAttributes(attribute.String("mysql.cmd", "StmtExec"))
	} else {
		span.SetAttributes(attribute.String("mysql.cmd", "TxStmtExec"))
	}
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
	span.End()
	return r, e
}
func (s *Stmt) Close() error {
	if len(s.stmts) == 0 {
		return nil
	} else if len(s.stmts) == 1 {
		var stmt *sql.Stmt
		for _, stmt = range s.stmts {
			break
		}
		return stmt.Close()
	} else {
		var e error
		wg := &sync.WaitGroup{}
		for _, v := range s.stmts {
			stmt := v
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := stmt.Close(); err != nil {
					e = err
				}
			}()
		}
		wg.Wait()
		return e
	}
}
func (o Operator) PrepareContext(ctx context.Context, query string) (*Stmt, error) {
	if len(o) == 0 {
		return nil, nil
	} else if len(o) == 1 {
		stmt, e := o[0].PrepareContext(ctx, query)
		if e != nil {
			return nil, e
		}
		return &Stmt{
			query: query,
			stmts: map[*cdb]*sql.Stmt{
				o[0]: stmt,
			},
		}, nil
	} else {
		var e error
		stmts := make(map[*cdb]*sql.Stmt)
		wg := &sync.WaitGroup{}
		for _, v := range o {
			db := v
			stmts[db] = nil
			wg.Add(1)
			go func() {
				defer wg.Done()
				stmt, err := db.PrepareContext(ctx, query)
				if err != nil {
					e = err
					return
				}
				stmts[db] = stmt
			}()
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
}
