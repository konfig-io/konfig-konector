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

// Package cloudcontrol holds helpers for the AWS Cloud Control API.
package cloudcontrol

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"

	"github.com/aws/smithy-go"
)

// IsNotFound reports whether err denotes a missing resource or request token.
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "ResourceNotFoundException", "RequestTokenNotFoundException":
			return true
		}
	}
	return false
}

// PatchOp is one RFC 6902 JSON Patch operation.
type PatchOp struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

// BuildPatch returns the JSON Patch that moves live to desired for every
// top-level property present in desired. Properties absent from desired are
// left untouched (they may be read-only or defaulted by the service), except
// those listed in removable, which are removed when absent from desired.
func BuildPatch(liveJSON, desiredJSON []byte, removable []string) ([]byte, error) {
	live := map[string]interface{}{}
	if len(liveJSON) > 0 {
		if err := json.Unmarshal(liveJSON, &live); err != nil {
			return nil, fmt.Errorf("parse live properties: %w", err)
		}
	}
	desired := map[string]interface{}{}
	if err := json.Unmarshal(desiredJSON, &desired); err != nil {
		return nil, fmt.Errorf("parse desired properties: %w", err)
	}
	var ops []PatchOp
	keys := make([]string, 0, len(desired))
	for k := range desired {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		lv, ok := live[k]
		if !ok {
			ops = append(ops, PatchOp{Op: "add", Path: "/" + escape(k), Value: desired[k]})
		} else if !reflect.DeepEqual(normalize(lv), normalize(desired[k])) {
			ops = append(ops, PatchOp{Op: "replace", Path: "/" + escape(k), Value: desired[k]})
		}
	}
	for _, k := range removable {
		if _, inDesired := desired[k]; !inDesired {
			if _, inLive := live[k]; inLive {
				ops = append(ops, PatchOp{Op: "remove", Path: "/" + escape(k)})
			}
		}
	}
	if len(ops) == 0 {
		return nil, nil
	}
	return json.Marshal(ops)
}

// normalize round-trips through JSON so numbers compare as float64 and
// nested maps/slices compare structurally.
func normalize(v interface{}) interface{} {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out interface{}
	_ = json.Unmarshal(b, &out)
	return out
}

func escape(k string) string {
	out := make([]byte, 0, len(k))
	for i := 0; i < len(k); i++ {
		switch k[i] {
		case '~':
			out = append(out, '~', '0')
		case '/':
			out = append(out, '~', '1')
		default:
			out = append(out, k[i])
		}
	}
	return string(out)
}
