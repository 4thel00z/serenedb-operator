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

// Package statements renders the SQL the operator runs against a SereneDB server.
package statements

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/4thel00z/serenedb-operator/internal/sqlexec"
)

var optionName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// DatabaseExists returns one row when the database exists.
func DatabaseExists(name string) (string, error) {
	lit, err := sqlexec.Literal(name)
	if err != nil {
		return "", err
	}
	return "SELECT 1 FROM pg_database WHERE datname = " + lit, nil
}

// CreateDatabase creates the database if it is missing.
func CreateDatabase(name string) string {
	return "CREATE DATABASE IF NOT EXISTS " + sqlexec.Identifier(name)
}

// DropDatabase drops the database.
func DropDatabase(name string) string {
	return "DROP DATABASE " + sqlexec.Identifier(name)
}

// RoleExists returns one row when the role exists.
func RoleExists(name string) (string, error) {
	lit, err := sqlexec.Literal(name)
	if err != nil {
		return "", err
	}
	return "SELECT 1 FROM pg_roles WHERE rolname = " + lit, nil
}

// RoleAttributes are the server-level attributes of a role.
type RoleAttributes struct {
	Login           bool
	Superuser       bool
	CreateDB        bool
	CreateRole      bool
	Inherit         bool
	ConnectionLimit *int32
	ValidUntil      string
	Password        *string
}

func (a RoleAttributes) render() (string, error) {
	parts := []string{
		flag(a.Login, "LOGIN"),
		flag(a.Superuser, "SUPERUSER"),
		flag(a.CreateDB, "CREATEDB"),
		flag(a.CreateRole, "CREATEROLE"),
		flag(a.Inherit, "INHERIT"),
	}
	if a.ConnectionLimit != nil {
		parts = append(parts, fmt.Sprintf("CONNECTION LIMIT %d", *a.ConnectionLimit))
	}
	if a.ValidUntil != "" {
		lit, err := sqlexec.Literal(a.ValidUntil)
		if err != nil {
			return "", err
		}
		parts = append(parts, "VALID UNTIL "+lit)
	}
	if a.Password != nil {
		lit, err := sqlexec.Literal(*a.Password)
		if err != nil {
			return "", err
		}
		parts = append(parts, "PASSWORD "+lit)
	}
	return strings.Join(parts, " "), nil
}

// CreateRole creates the role with its attributes.
func CreateRole(name string, attrs RoleAttributes) (string, error) {
	rendered, err := attrs.render()
	if err != nil {
		return "", err
	}
	return "CREATE ROLE " + sqlexec.Identifier(name) + " " + rendered, nil
}

// AlterRole applies the attributes to an existing role.
func AlterRole(name string, attrs RoleAttributes) (string, error) {
	rendered, err := attrs.render()
	if err != nil {
		return "", err
	}
	return "ALTER ROLE " + sqlexec.Identifier(name) + " " + rendered, nil
}

// DropRole drops the role.
func DropRole(name string) string {
	return "DROP ROLE " + sqlexec.Identifier(name)
}

// RoleMemberships lists the roles the named role is a member of, one per row.
func RoleMemberships(name string) (string, error) {
	lit, err := sqlexec.Literal(name)
	if err != nil {
		return "", err
	}
	return "SELECT r.rolname FROM pg_auth_members am JOIN pg_roles r ON r.oid = am.roleid " +
		"JOIN pg_roles m ON m.oid = am.member WHERE m.rolname = " + lit + " ORDER BY 1", nil
}

// GrantMembership adds member to group.
func GrantMembership(group, member string) string {
	return "GRANT " + sqlexec.Identifier(group) + " TO " + sqlexec.Identifier(member)
}

// RevokeMembership removes member from group.
func RevokeMembership(group, member string) string {
	return "REVOKE " + sqlexec.Identifier(group) + " FROM " + sqlexec.Identifier(member)
}

// SecretExists returns one row when the server secret exists.
func SecretExists(name string) (string, error) {
	lit, err := sqlexec.Literal(name)
	if err != nil {
		return "", err
	}
	return "SELECT 1 FROM sdb_secrets() WHERE name = " + lit, nil
}

// CreateSecret creates or replaces a persistent server secret. Options are rendered in sorted key order.
func CreateSecret(name, secretType, scope string, options map[string]string) (string, error) {
	keys := make([]string, 0, len(options))
	for k := range options {
		if !optionName.MatchString(k) {
			return "", fmt.Errorf("option %q is not a valid option name", k)
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := []string{"TYPE " + secretType}
	if scope != "" {
		lit, err := sqlexec.Literal(scope)
		if err != nil {
			return "", err
		}
		parts = append(parts, "SCOPE "+lit)
	}
	for _, k := range keys {
		lit, err := sqlexec.Literal(options[k])
		if err != nil {
			return "", err
		}
		parts = append(parts, strings.ToUpper(k)+" "+lit)
	}
	return "CREATE OR REPLACE PERSISTENT SECRET " + sqlexec.Identifier(name) + " (" + strings.Join(parts, ", ") + ")", nil
}

// DropSecret drops the server secret.
func DropSecret(name string) string {
	return "DROP SECRET " + sqlexec.Identifier(name)
}

func flag(on bool, word string) string {
	if on {
		return word
	}
	return "NO" + word
}
