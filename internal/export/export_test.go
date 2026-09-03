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

package export

import "testing"

func TestCRName(t *testing.T) {
	cases := map[string]string{
		"my-queue":                    "my-queue",
		"MyQueue.fifo":                "myqueue-fifo",
		"/platformdev/kk-smoke/param": "platformdev-kk-smoke-param",
		"arn:aws:iam::1:role/My_Role": "arn-aws-iam-1-role-my-role",
		"a__b":                        "a-b",
		"---":                         "unnamed",
		"":                            "unnamed",
		"UPPER":                       "upper",
	}
	for in, want := range cases {
		if got := CRName(in); got != want {
			t.Errorf("CRName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTagMapFiltersAWSTags(t *testing.T) {
	in := map[string]string{
		"env":                    "dev",
		"aws:cloudformation:sid": "x",
	}
	out := TagMap(in)
	if _, ok := out["aws:cloudformation:sid"]; ok {
		t.Error("aws: tag not filtered")
	}
	if out["env"] != "dev" {
		t.Error("user tag lost")
	}
	if TagMap(map[string]string{"aws:only": "x"}) != nil {
		t.Error("all-system tag map should collapse to nil")
	}
}

func TestRefIndex(t *testing.T) {
	idx := NewRefIndex()
	idx.Add("vpc-123", "my-vpc")
	idx.Add("", "ignored")
	if n, ok := idx.Lookup("vpc-123"); !ok || n != "my-vpc" {
		t.Errorf("lookup = %q,%v", n, ok)
	}
	if _, ok := idx.Lookup("vpc-999"); ok {
		t.Error("unexpected hit")
	}
}
