package exasolclient

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// ReadCache holds whole system views for Read. Terraform reads every resource
// of a plan, refresh or import with one provider instance, and each lookup on
// an EXA_DBA_* view costs about the same whether it returns one row or all of
// them. So one query per view replaces one query per resource. Terraform
// writes in another provider instance (the apply walk configures a new one),
// so Clear on every write is a safeguard for a Read after a write.
//
// Limit: whole views are held in memory, so a database with hundreds of
// thousands of object grants loads all of them even for a -target plan. Load
// only above a resource count, or filter the view, if that ever matters.
type ReadCache struct {
	mu    sync.Mutex
	views map[string]*cachedView
}

type cachedView struct {
	mu     sync.Mutex
	loaded bool
	err    error
	rows   map[string][][]sql.NullString
}

func NewReadCache() *ReadCache { return &ReadCache{views: map[string]*cachedView{}} }

// Rows returns the rows of query whose first len(key) columns equal key, like
// "WHERE col1 = ? AND col2 = ?" would. A NULL in a key column matches nothing,
// as in SQL. Calls with another number of keys use their own copy of the view.
// The rows are shared with other callers and must not be modified.
//
// The first caller of a view loads it while the others wait. A failed load is
// kept until Clear, so parallel Reads fail with that error instead of each
// loading the view again; a cancelled or timed-out load is not kept, because
// it belongs to the caller's context. A nil cache loads the view on every call.
func (c *ReadCache) Rows(ctx context.Context, db *sql.DB, query string, key ...string) ([][]sql.NullString, error) {
	if c == nil {
		rows, err := loadView(ctx, db, query, len(key))
		return rows[strings.Join(key, "\x00")], err
	}
	v := c.view(fmt.Sprint(len(key), "\x00", query))
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.loaded {
		v.rows, v.err = loadView(ctx, db, query, len(key))
		v.loaded = !errors.Is(v.err, context.Canceled) && !errors.Is(v.err, context.DeadlineExceeded)
	}
	if v.err != nil {
		return nil, v.err
	}
	return v.rows[strings.Join(key, "\x00")], nil
}

func (c *ReadCache) view(id string) *cachedView {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.views[id]
	if !ok {
		v = &cachedView{}
		c.views[id] = v
	}
	return v
}

// Clear drops all cached views. A load running during Clear fills a view that
// is no longer in the cache, so its rows only reach callers that asked for the
// view before Clear.
func (c *ReadCache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.views = map[string]*cachedView{}
}

func loadView(ctx context.Context, db *sql.DB, query string, nkey int) (map[string][][]sql.NullString, error) {
	rs, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rs.Close() }()
	cols, err := rs.Columns()
	if err != nil {
		return nil, err
	}
	if nkey > len(cols) {
		return nil, fmt.Errorf("read cache: %d key columns for a query with %d columns", nkey, len(cols))
	}
	out := map[string][][]sql.NullString{}
	for rs.Next() {
		row := make([]sql.NullString, len(cols))
		dest := make([]any, len(cols))
		for i := range row {
			dest[i] = &row[i]
		}
		if err := rs.Scan(dest...); err != nil {
			return nil, err
		}
		key, ok := rowKey(row[:nkey])
		if ok {
			out[key] = append(out[key], row)
		}
	}
	return out, rs.Err()
}

func rowKey(cols []sql.NullString) (string, bool) {
	parts := make([]string, len(cols))
	for i, c := range cols {
		if !c.Valid {
			return "", false
		}
		parts[i] = c.String
	}
	return strings.Join(parts, "\x00"), true
}
