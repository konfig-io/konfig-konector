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

package controller

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Multi-account guard: controllers must never assume the operator's own
// region or account. Region and account come from the provider scope on the
// context (applied by the SDK wrappers) or from the resource spec. Anything
// listed here needs a deliberate, reviewed exception.
func TestControllersDoNotAssumeOperatorRegionOrAccount(t *testing.T) {
	forbidden := []*regexp.Regexp{
		regexp.MustCompile(`os\.Getenv\("AWS_REGION"\)`),
		regexp.MustCompile(`AWS_DEFAULT_REGION`),
		regexp.MustCompile(`\.Config\.Region`),
		regexp.MustCompile(`config\.LoadDefaultConfig`),
		regexp.MustCompile(`\bNewFromConfig\(`),
		regexp.MustCompile(`r\.AccountID\b`),
	}
	// file -> allowed patterns (reviewed exceptions)
	allow := map[string][]string{
		"s3bucket_controller.go":    {`os.Getenv("AWS_REGION")`}, // fallback only after the scope region
		"iampolicy_controller.go":   {`r.AccountID`},             // fallback inside accountID(ctx) only
		"awsprovider_controller.go": {},
	}
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, _ := os.ReadFile(f)
		for i, line := range strings.Split(string(src), "\n") {
			for _, re := range forbidden {
				if !re.MatchString(line) {
					continue
				}
				ok := false
				for _, a := range allow[f] {
					if strings.Contains(line, a) {
						ok = true
					}
				}
				if !ok {
					t.Errorf("%s:%d assumes the operator's region/account: %s", f, i+1, strings.TrimSpace(line))
				}
			}
		}
	}
}
