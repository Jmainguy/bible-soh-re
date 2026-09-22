package main

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/lib/pq"
)

// sqlBackend keeps the local SQLite and production PostgreSQL queries equivalent.
type sqlBackend struct {
	*sql.DB
	postgres bool
}

func (b *sqlBackend) query(q string) string {
	if !b.postgres {
		return q
	}
	q = strings.ReplaceAll(q, "INTEGER PRIMARY KEY AUTOINCREMENT", "SERIAL PRIMARY KEY")
	q = strings.ReplaceAll(q, "DATETIME", "TIMESTAMPTZ")
	q = strings.ReplaceAll(q, "GROUP_CONCAT(user_id)", "STRING_AGG(user_id::text, ',')")
	q = strings.ReplaceAll(q, "ADD COLUMN ", "ADD COLUMN IF NOT EXISTS ")
	var out strings.Builder
	n := 0
	for _, c := range q {
		if c == '?' {
			n++
			fmt.Fprintf(&out, "$%d", n)
		} else {
			out.WriteRune(c)
		}
	}
	return out.String()
}
func (b *sqlBackend) Exec(q string, args ...any) (sql.Result, error) {
	return b.DB.Exec(b.query(q), args...)
}
func (b *sqlBackend) Query(q string, args ...any) (*sql.Rows, error) {
	return b.DB.Query(b.query(q), args...)
}
func (b *sqlBackend) QueryRow(q string, args ...any) *sql.Row {
	return b.DB.QueryRow(b.query(q), args...)
}
func (d *Database) insert(q string, args ...any) (int64, error) {
	if d.db.postgres {
		var id int64
		err := d.db.QueryRow(q+" RETURNING id", args...).Scan(&id)
		return id, err
	}
	result, err := d.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
