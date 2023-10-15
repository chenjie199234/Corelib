package mysql

import (
	"context"
	"database/sql"
	"math/rand"
	"sync"

	_ "github.com/chenjie199234/Corelib/log"
	"github.com/chenjie199234/Corelib/log/trace"
)

type cdb struct {
	db   *sql.DB
	addr string
	name string
}

func (c *cdb) Stats() sql.DBStats {
	return c.db.Stats()
}
func (c *cdb) PingContext(ctx context.Context) error {
	ctx, span := trace.NewSpan(ctx, "Corelib.Mysql", trace.Client, nil)
	span.GetSelfSpanData().SetStateKV("mysql", c.name)
	span.GetSelfSpanData().SetStateKV("host", c.addr)
	span.GetSelfSpanData().SetStateKV("cmd", "Ping")
	e := c.db.PingContext(ctx)
	span.Finish(e)
	return e
}
func (c *cdb) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	ctx, span := trace.NewSpan(ctx, "Corelib.Mysql", trace.Client, nil)
	span.GetSelfSpanData().SetStateKV("mysql", c.name)
	span.GetSelfSpanData().SetStateKV("host", c.addr)
	span.GetSelfSpanData().SetStateKV("cmd", "QueryRow")
	r := c.db.QueryRowContext(ctx, query, args...)
	span.Finish(r.Err())
	return r
}
func (c *cdb) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	ctx, span := trace.NewSpan(ctx, "Corelib.Mysql", trace.Client, nil)
	span.GetSelfSpanData().SetStateKV("mysql", c.name)
	span.GetSelfSpanData().SetStateKV("host", c.addr)
	span.GetSelfSpanData().SetStateKV("cmd", "Query")
	rs, e := c.db.QueryContext(ctx, query, args...)
	span.Finish(e)
	return rs, e
}
func (c *cdb) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	ctx, span := trace.NewSpan(ctx, "Corelib.Mysql", trace.Client, nil)
	span.GetSelfSpanData().SetStateKV("mysql", c.name)
	span.GetSelfSpanData().SetStateKV("host", c.addr)
	span.GetSelfSpanData().SetStateKV("cmd", "Exec")
	r, e := c.db.ExecContext(ctx, query, args...)
	span.Finish(e)
	return r, e
}
func (c *cdb) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	ctx, span := trace.NewSpan(ctx, "Corelib.Mysql", trace.Client, nil)
	span.GetSelfSpanData().SetStateKV("mysql", c.name)
	span.GetSelfSpanData().SetStateKV("host", c.addr)
	span.GetSelfSpanData().SetStateKV("cmd", "Begin")
	tx, e := c.db.BeginTx(ctx, opts)
	span.Finish(e)
	return tx, e
}
func (c *cdb) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	ctx, span := trace.NewSpan(ctx, "Corelib.Mysql", trace.Client, nil)
	span.GetSelfSpanData().SetStateKV("mysql", c.name)
	span.GetSelfSpanData().SetStateKV("host", c.addr)
	span.GetSelfSpanData().SetStateKV("cmd", "Prepare")
	stmt, e := c.db.PrepareContext(ctx, query)
	span.Finish(e)
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
	ctx, span := trace.NewSpan(ctx, "Corelib.Mysql", trace.Client, nil)
	span.GetSelfSpanData().SetStateKV("mysql", t.db.name)
	span.GetSelfSpanData().SetStateKV("host", t.db.addr)
	span.GetSelfSpanData().SetStateKV("cmd", "TxQueryRow")
	r := t.t.QueryRowContext(ctx, query, args...)
	span.Finish(r.Err())
	return r
}
func (t *Tx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	ctx, span := trace.NewSpan(ctx, "Corelib.Mysql", trace.Client, nil)
	span.GetSelfSpanData().SetStateKV("mysql", t.db.name)
	span.GetSelfSpanData().SetStateKV("host", t.db.addr)
	span.GetSelfSpanData().SetStateKV("cmd", "TxQuery")
	rs, e := t.t.QueryContext(ctx, query, args...)
	span.Finish(e)
	return rs, e
}
func (t *Tx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	ctx, span := trace.NewSpan(ctx, "Corelib.Mysql", trace.Client, nil)
	span.GetSelfSpanData().SetStateKV("mysql", t.db.name)
	span.GetSelfSpanData().SetStateKV("host", t.db.addr)
	span.GetSelfSpanData().SetStateKV("cmd", "TxExec")
	r, e := t.t.ExecContext(ctx, query, args...)
	span.Finish(e)
	return r, e
}

// the returned stmt don't need to close manually,it will be closed by commit or rollback
func (t *Tx) PrepareContext(ctx context.Context, query string) (*Stmt, error) {
	ctx, span := trace.NewSpan(ctx, "Corelib.Mysql", trace.Client, nil)
	span.GetSelfSpanData().SetStateKV("mysql", t.db.name)
	span.GetSelfSpanData().SetStateKV("host", t.db.addr)
	span.GetSelfSpanData().SetStateKV("cmd", "TxPrepare")
	newstmt, e := t.t.PrepareContext(ctx, query)
	span.Finish(e)
	return &Stmt{
		tx:    t,
		query: query,
		stmts: map[*cdb]*sql.Stmt{t.db: newstmt},
	}, nil
}
func (t *Tx) Commit(ctx context.Context) error {
	ctx, span := trace.NewSpan(ctx, "Corelib.Mysql", trace.Client, nil)
	span.GetSelfSpanData().SetStateKV("mysql", t.db.name)
	span.GetSelfSpanData().SetStateKV("host", t.db.addr)
	span.GetSelfSpanData().SetStateKV("cmd", "Commit")
	e := t.t.Commit()
	span.Finish(e)
	return e
}
func (t *Tx) Rollback(ctx context.Context) error {
	ctx, span := trace.NewSpan(ctx, "Corelib.Mysql", trace.Client, nil)
	span.GetSelfSpanData().SetStateKV("mysql", t.db.name)
	span.GetSelfSpanData().SetStateKV("host", t.db.addr)
	span.GetSelfSpanData().SetStateKV("cmd", "Rollback")
	e := t.t.Rollback()
	span.Finish(e)
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
	ctx, span := trace.NewSpan(ctx, "Corelib.Mysql", trace.Client, nil)
	span.GetSelfSpanData().SetStateKV("mysql", db.name)
	span.GetSelfSpanData().SetStateKV("host", db.addr)
	if s.tx == nil {
		span.GetSelfSpanData().SetStateKV("cmd", "StmtQueryRow")
	} else {
		span.GetSelfSpanData().SetStateKV("cmd", "TxStmtQueryRow")
	}
	r := stmt.QueryRowContext(ctx, args...)
	span.Finish(r.Err())
	return r
}
func (s *Stmt) QueryContext(ctx context.Context, args ...any) (*sql.Rows, error) {
	var stmt *sql.Stmt
	var db *cdb
	for db, stmt = range s.stmts {
		//this is random
		break
	}
	ctx, span := trace.NewSpan(ctx, "Corelib.Mysql", trace.Client, nil)
	span.GetSelfSpanData().SetStateKV("mysql", db.name)
	span.GetSelfSpanData().SetStateKV("host", db.addr)
	if s.tx == nil {
		span.GetSelfSpanData().SetStateKV("cmd", "StmtQuery")
	} else {
		span.GetSelfSpanData().SetStateKV("cmd", "TxStmtQuery")
	}
	rs, e := stmt.QueryContext(ctx, args...)
	span.Finish(e)
	return rs, e
}
func (s *Stmt) ExecContext(ctx context.Context, args ...any) (sql.Result, error) {
	var stmt *sql.Stmt
	var db *cdb
	for db, stmt = range s.stmts {
		//this is random
		break
	}
	ctx, span := trace.NewSpan(ctx, "Corelib.Mysql", trace.Client, nil)
	span.GetSelfSpanData().SetStateKV("mysql", db.name)
	span.GetSelfSpanData().SetStateKV("host", db.addr)
	if s.tx == nil {
		span.GetSelfSpanData().SetStateKV("cmd", "StmtExec")
	} else {
		span.GetSelfSpanData().SetStateKV("cmd", "TxStmtExec")
	}
	r, e := stmt.ExecContext(ctx, args...)
	span.Finish(e)
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
