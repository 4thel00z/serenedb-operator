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

package statements

import (
	"testing"

	"k8s.io/utils/ptr"
)

func TestDatabaseStatements(t *testing.T) {
	q, err := DatabaseExists("it's")
	if err != nil || q != "SELECT 1 FROM pg_database WHERE datname = 'it''s'" {
		t.Fatalf("%q %v", q, err)
	}
	if got := CreateDatabase("we-ird"); got != `CREATE DATABASE IF NOT EXISTS "we-ird"` {
		t.Fatal(got)
	}
	if got := DropDatabase("x"); got != `DROP DATABASE "x"` {
		t.Fatal(got)
	}
}

func TestRoleStatements(t *testing.T) {
	attrs := RoleAttributes{Login: true, Inherit: true, ConnectionLimit: ptr.To[int32](5), ValidUntil: "2030-01-01", Password: ptr.To("p'w")}
	got, err := CreateRole("app", attrs)
	if err != nil {
		t.Fatal(err)
	}
	want := `CREATE ROLE "app" LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE INHERIT CONNECTION LIMIT 5 VALID UNTIL '2030-01-01' PASSWORD 'p''w'`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	got, err = AlterRole("app", RoleAttributes{Superuser: true})
	if err != nil || got != `ALTER ROLE "app" NOLOGIN SUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT` {
		t.Fatalf("%q %v", got, err)
	}
	if GrantMembership("readers", "app") != `GRANT "readers" TO "app"` || RevokeMembership("readers", "app") != `REVOKE "readers" FROM "app"` {
		t.Fatal("membership statements")
	}
	q, err := RoleMemberships("app")
	if err != nil || q == "" {
		t.Fatal(err)
	}
}

func TestSecretStatements(t *testing.T) {
	got, err := CreateSecret("s3main", "s3", "s3://bucket/", map[string]string{"SECRET": "sec", "key_id": "id", "REGION": "eu"})
	if err != nil {
		t.Fatal(err)
	}
	want := `CREATE OR REPLACE PERSISTENT SECRET "s3main" (TYPE s3, SCOPE 's3://bucket/', REGION 'eu', SECRET 'sec', KEY_ID 'id')`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	if _, err := CreateSecret("x", "s3", "", map[string]string{"bad key": "v"}); err == nil {
		t.Fatal("invalid option names must be rejected")
	}
	if DropSecret("x") != `DROP SECRET "x"` {
		t.Fatal("drop")
	}
}

func TestCreateSecretOpenAI(t *testing.T) {
	got, err := CreateSecret("embeddings", "openai", "", map[string]string{"api_key": "sk-1", "base_url": "https://emb.internal/v1"})
	if err != nil {
		t.Fatal(err)
	}
	want := `CREATE OR REPLACE PERSISTENT SECRET "embeddings" (TYPE openai, API_KEY 'sk-1', BASE_URL 'https://emb.internal/v1')`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}
