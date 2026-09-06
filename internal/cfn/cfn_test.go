package cfn

import (
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

type tag struct {
	Key   string `json:"key" cfn:"Key"`
	Value string `json:"value" cfn:"Value"`
}
type nested struct {
	Enabled *bool  `json:"enabled,omitempty" cfn:"Enabled"`
	Mode    string `json:"mode,omitempty" cfn:"Mode"`
}
type base struct {
	Identifier string `json:"identifier,omitempty"`
}
type spec struct {
	Name      string                `json:"name" cfn:"Name"`
	Retention *int64                `json:"retention,omitempty" cfn:"RetentionInDays"`
	Ratio     *float64              `json:"ratio,omitempty" cfn:"Ratio"`
	Tags      []tag                 `json:"tags,omitempty" cfn:"Tags"`
	Config    *nested               `json:"config,omitempty" cfn:"Config"`
	Labels    map[string]string     `json:"labels,omitempty" cfn:"Labels"`
	Raw       *apiextensionsv1.JSON `json:"raw,omitempty" cfn:"Policy"`
	Ignored   string                `json:"ignored,omitempty"`
}
type status struct {
	base `json:",inline"`
	Arn  *string `json:"arn,omitempty" cfn:"Arn"`
	Zone *nested `json:"zone,omitempty" cfn:"Zone"`
}

func TestRoundTrip(t *testing.T) {
	seven := int64(7)
	tr := true
	s := &spec{Name: "lg", Retention: &seven, Tags: []tag{{Key: "k", Value: "v"}}, Config: &nested{Enabled: &tr},
		Labels: map[string]string{"a": "b"}, Raw: &apiextensionsv1.JSON{Raw: []byte(`{"Version":"2012-10-17"}`)}, Ignored: "x"}
	out, err := ToDesiredState(s)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"Config":{"Enabled":true},"Labels":{"a":"b"},"Name":"lg","Policy":{"Version":"2012-10-17"},"RetentionInDays":7,"Tags":[{"Key":"k","Value":"v"}]}`
	if string(out) != want {
		t.Fatalf("got  %s\nwant %s", out, want)
	}
	var back spec
	if err := FromProperties(out, &back); err != nil {
		t.Fatal(err)
	}
	if back.Name != "lg" || *back.Retention != 7 || len(back.Tags) != 1 || !*back.Config.Enabled || back.Labels["a"] != "b" || string(back.Raw.Raw) != `{"Version":"2012-10-17"}` {
		t.Fatalf("round trip mismatch: %+v", back)
	}
	var st status
	if err := FromProperties([]byte(`{"Arn":"arn:x","Zone":{"Mode":"m"},"Unknown":1}`), &st); err != nil {
		t.Fatal(err)
	}
	if st.Arn == nil || *st.Arn != "arn:x" || st.Zone.Mode != "m" {
		t.Fatalf("status fill mismatch: %+v", st)
	}
	if e, _ := ToDesiredState(&spec{}); string(e) != "{}" {
		t.Fatalf("empty spec should be {}: %s", e)
	}
}
