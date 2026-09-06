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

// Package cfn converts between konfig-konector typed specs (lowerCamel JSON
// with `cfn:"PropertyName"` struct tags) and CloudFormation/Cloud Control
// property documents (PascalCase property names).
package cfn

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1alpha1 "github.com/konfig-io/konfig-konector/api/v1alpha1"
)

// CloudControlObject is implemented by every generated Cloud Control-backed
// kind (hack/gen-cloudcontrol). The shared engine in internal/controller and
// the generic exporter drive them through this interface.
type CloudControlObject interface {
	client.Object
	GetProviderRef() *awsv1alpha1.ProviderRef
	// CloudControlTypeName is the CloudFormation type, e.g. AWS::Logs::MetricStream.
	CloudControlTypeName() string
	// CloudControlSpec returns a pointer to the typed spec (cfn-tagged) that
	// becomes the DesiredState document.
	CloudControlSpec() interface{}
	// CloudControlStatusRef returns the shared status block.
	CloudControlStatusRef() *awsv1alpha1.CloudControlStatus
	// CloudControlObserved returns a pointer to the typed status so read-only
	// properties from GetResource can be filled in.
	CloudControlObserved() interface{}
}

var jsonType = reflect.TypeOf(apiextensionsv1.JSON{})

// ToDesiredState renders v (a pointer to a generated spec struct) as the
// Cloud Control DesiredState JSON document. Fields without a cfn tag are
// skipped; unset optional values are omitted.
func ToDesiredState(v interface{}) ([]byte, error) {
	m, err := toCFN(reflect.ValueOf(v))
	if err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]interface{}{}
	}
	return json.Marshal(m)
}

// toCFN returns nil (with nil error) for values that should be omitted.
func toCFN(v reflect.Value) (interface{}, error) {
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil, nil
		}
		v = v.Elem()
	}
	if v.Type() == jsonType {
		raw := v.Interface().(apiextensionsv1.JSON).Raw
		if len(raw) == 0 {
			return nil, nil
		}
		var out interface{}
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, fmt.Errorf("invalid JSON value: %w", err)
		}
		return out, nil
	}
	switch v.Kind() {
	case reflect.Struct:
		out := map[string]interface{}{}
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name := f.Tag.Get("cfn")
			if name == "" {
				if f.Anonymous {
					inner, err := toCFN(v.Field(i))
					if err != nil {
						return nil, err
					}
					if im, ok := inner.(map[string]interface{}); ok {
						for k, val := range im {
							out[k] = val
						}
					}
				}
				continue
			}
			val, err := toCFN(v.Field(i))
			if err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			if val != nil {
				out[name] = val
			}
		}
		if len(out) == 0 {
			return nil, nil
		}
		return out, nil
	case reflect.Slice:
		if v.IsNil() || v.Len() == 0 {
			return nil, nil
		}
		out := make([]interface{}, 0, v.Len())
		for i := 0; i < v.Len(); i++ {
			val, err := toCFN(v.Index(i))
			if err != nil {
				return nil, err
			}
			if val == nil {
				val = zeroFor(v.Index(i))
			}
			out = append(out, val)
		}
		return out, nil
	case reflect.Map:
		if v.IsNil() || v.Len() == 0 {
			return nil, nil
		}
		out := map[string]interface{}{}
		for _, k := range v.MapKeys() {
			val, err := toCFN(v.MapIndex(k))
			if err != nil {
				return nil, err
			}
			if val == nil {
				val = zeroFor(v.MapIndex(k))
			}
			out[fmt.Sprint(k.Interface())] = val
		}
		return out, nil
	case reflect.String:
		if v.String() == "" {
			return nil, nil
		}
		return v.String(), nil
	case reflect.Bool:
		return v.Bool(), nil
	case reflect.Int, reflect.Int32, reflect.Int64:
		return v.Int(), nil
	case reflect.Float32, reflect.Float64:
		return v.Float(), nil
	}
	return nil, fmt.Errorf("unsupported kind %s", v.Kind())
}

// zeroFor keeps explicit empty elements inside collections (an empty string
// in a list is still an element).
func zeroFor(v reflect.Value) interface{} {
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.String:
		return ""
	case reflect.Struct, reflect.Map:
		return map[string]interface{}{}
	case reflect.Slice:
		return []interface{}{}
	}
	return nil
}

// FromProperties fills target (pointer to a generated spec or status struct)
// from a Cloud Control properties document. Properties without a matching
// cfn-tagged field are ignored; type mismatches are reported.
func FromProperties(properties []byte, target interface{}) error {
	var doc interface{}
	if len(properties) == 0 {
		return nil
	}
	if err := json.Unmarshal(properties, &doc); err != nil {
		return fmt.Errorf("parse properties: %w", err)
	}
	return fromCFN(doc, reflect.ValueOf(target))
}

func fromCFN(src interface{}, dst reflect.Value) error {
	if src == nil {
		return nil
	}
	if dst.Kind() == reflect.Ptr {
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		return fromCFN(src, dst.Elem())
	}
	if dst.Type() == jsonType {
		raw, err := json.Marshal(src)
		if err != nil {
			return err
		}
		dst.Set(reflect.ValueOf(apiextensionsv1.JSON{Raw: raw}))
		return nil
	}
	switch dst.Kind() {
	case reflect.Struct:
		m, ok := src.(map[string]interface{})
		if !ok {
			return fmt.Errorf("expected object for %s, got %T", dst.Type(), src)
		}
		t := dst.Type()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name := f.Tag.Get("cfn")
			if name == "" {
				if f.Anonymous {
					if err := fromCFN(src, dst.Field(i)); err != nil {
						return err
					}
				}
				continue
			}
			val, ok := m[name]
			if !ok || val == nil {
				continue
			}
			if err := fromCFN(val, dst.Field(i)); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
		return nil
	case reflect.Slice:
		arr, ok := src.([]interface{})
		if !ok {
			return fmt.Errorf("expected array, got %T", src)
		}
		out := reflect.MakeSlice(dst.Type(), len(arr), len(arr))
		for i, item := range arr {
			if err := fromCFN(item, out.Index(i)); err != nil {
				return err
			}
		}
		dst.Set(out)
		return nil
	case reflect.Map:
		m, ok := src.(map[string]interface{})
		if !ok {
			return fmt.Errorf("expected object for map, got %T", src)
		}
		out := reflect.MakeMap(dst.Type())
		for k, item := range m {
			ev := reflect.New(dst.Type().Elem()).Elem()
			if err := fromCFN(item, ev); err != nil {
				return err
			}
			out.SetMapIndex(reflect.ValueOf(k), ev)
		}
		dst.Set(out)
		return nil
	case reflect.String:
		switch s := src.(type) {
		case string:
			dst.SetString(s)
		case float64:
			dst.SetString(strconv.FormatFloat(s, 'f', -1, 64))
		case bool:
			dst.SetString(strconv.FormatBool(s))
		default:
			b, _ := json.Marshal(s)
			dst.SetString(string(b))
		}
		return nil
	case reflect.Bool:
		switch b := src.(type) {
		case bool:
			dst.SetBool(b)
		case string:
			dst.SetBool(b == "true")
		default:
			return fmt.Errorf("expected bool, got %T", src)
		}
		return nil
	case reflect.Int, reflect.Int32, reflect.Int64:
		switch n := src.(type) {
		case float64:
			dst.SetInt(int64(n))
		case string:
			i, err := strconv.ParseInt(n, 10, 64)
			if err != nil {
				return err
			}
			dst.SetInt(i)
		default:
			return fmt.Errorf("expected integer, got %T", src)
		}
		return nil
	case reflect.Float32, reflect.Float64:
		switch n := src.(type) {
		case float64:
			dst.SetFloat(n)
		case string:
			f, err := strconv.ParseFloat(n, 64)
			if err != nil {
				return err
			}
			dst.SetFloat(f)
		default:
			return fmt.Errorf("expected number, got %T", src)
		}
		return nil
	}
	return fmt.Errorf("unsupported target kind %s", dst.Kind())
}
