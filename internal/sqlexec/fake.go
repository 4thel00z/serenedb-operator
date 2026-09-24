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

package sqlexec

import (
	"context"
	"sync"
)

// Fake records every statement and answers queries from a table of canned rows. It stands in for a
// server in envtest.
type Fake struct {
	mu         sync.Mutex
	Statements []string
	Rows       map[string][][]string
	ConnectErr error
	ExecErr    error
	Connects   int
}

// NewFake returns an empty Fake.
func NewFake() *Fake {
	return &Fake{Rows: map[string][][]string{}}
}

// Connector returns a Connector that hands out sessions bound to this Fake.
func (f *Fake) Connector() Connector {
	return func(_ context.Context, _ Target) (Session, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.Connects++
		if f.ConnectErr != nil {
			return nil, f.ConnectErr
		}
		return fakeSession{f}, nil
	}
}

// Answer sets the rows a query returns.
func (f *Fake) Answer(query string, rows [][]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Rows[query] = rows
}

// Executed reports whether a statement was run.
func (f *Fake) Executed(statement string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.Statements {
		if s == statement {
			return true
		}
	}
	return false
}

// Recorded returns a copy of the statements run so far.
func (f *Fake) Recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.Statements...)
}

type fakeSession struct {
	f *Fake
}

func (s fakeSession) Exec(_ context.Context, statement string) error {
	s.f.mu.Lock()
	defer s.f.mu.Unlock()
	if s.f.ExecErr != nil {
		return s.f.ExecErr
	}
	s.f.Statements = append(s.f.Statements, statement)
	return nil
}

func (s fakeSession) Query(_ context.Context, query string) ([][]string, error) {
	s.f.mu.Lock()
	defer s.f.mu.Unlock()
	s.f.Statements = append(s.f.Statements, query)
	return s.f.Rows[query], nil
}

func (s fakeSession) Close(context.Context) error {
	return nil
}
