/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package sqlexec runs SQL against a SereneDB server over the pg-wire protocol.
package sqlexec

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Target is one server to connect to as the superuser.
type Target struct {
	Host     string
	Port     int32
	User     string
	Password string
	TLS      bool
}

// Session runs statements on one connection. Close releases it.
type Session interface {
	Exec(ctx context.Context, statement string) error
	Query(ctx context.Context, query string) ([][]string, error)
	Close(ctx context.Context) error
}

// Connector opens a Session for a Target.
type Connector func(ctx context.Context, target Target) (Session, error)

type pgxSession struct {
	conn *pgx.Conn
}

// Connect opens one pg-wire connection to the target.
func Connect(ctx context.Context, target Target) (Session, error) {
	sslmode := "disable"
	if target.TLS {
		sslmode = "require"
	}
	cfg, err := pgx.ParseConfig(fmt.Sprintf("host=%s port=%d user=%s dbname=postgres sslmode=%s connect_timeout=10",
		target.Host, target.Port, target.User, sslmode))
	if err != nil {
		return nil, err
	}
	cfg.Password = target.Password
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &pgxSession{conn: conn}, nil
}

func (s *pgxSession) Exec(ctx context.Context, statement string) error {
	_, err := s.conn.Exec(ctx, statement)
	return err
}

func (s *pgxSession) Query(ctx context.Context, query string) ([][]string, error) {
	rows, err := s.conn.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]string
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := make([]string, len(values))
		for i, v := range values {
			row[i] = fmt.Sprint(v)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *pgxSession) Close(ctx context.Context) error {
	return s.conn.Close(ctx)
}

// Identifier quotes a SQL identifier such as a database, role or secret name.
func Identifier(name string) string {
	return pgx.Identifier{name}.Sanitize()
}

// Literal quotes a SQL string literal. NUL bytes are rejected because the wire protocol cannot carry them.
func Literal(value string) (string, error) {
	if strings.ContainsRune(value, 0) {
		return "", fmt.Errorf("value contains a NUL byte")
	}
	return "'" + strings.ReplaceAll(value, "'", "''") + "'", nil
}
