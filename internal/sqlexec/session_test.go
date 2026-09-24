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

import "testing"

func TestIdentifierQuotesAndEscapes(t *testing.T) {
	if got := Identifier("app"); got != `"app"` {
		t.Fatalf("got %s", got)
	}
	if got := Identifier(`x"; DROP ROLE postgres; --`); got != `"x""; DROP ROLE postgres; --"` {
		t.Fatalf("got %s", got)
	}
}

func TestLiteralEscapesQuotesAndRejectsNUL(t *testing.T) {
	got, err := Literal("it's")
	if err != nil || got != "'it''s'" {
		t.Fatalf("got %q err %v", got, err)
	}
	if _, err := Literal("a\x00b"); err == nil {
		t.Fatal("NUL must be rejected")
	}
}
