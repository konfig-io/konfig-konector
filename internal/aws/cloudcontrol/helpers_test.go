package cloudcontrol

import "testing"

func TestBuildPatch(t *testing.T) {
	live := []byte(`{"LogGroupName":"a","RetentionInDays":7,"Arn":"arn:x","Tags":[{"Key":"k","Value":"v"}]}`)
	desired := []byte(`{"LogGroupName":"a","RetentionInDays":14,"KmsKeyId":"arn:kms"}`)
	p, err := BuildPatch(live, desired, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"op":"add","path":"/KmsKeyId","value":"arn:kms"},{"op":"replace","path":"/RetentionInDays","value":14}]`
	if string(p) != want {
		t.Fatalf("got %s\nwant %s", p, want)
	}
	if p, _ := BuildPatch(live, []byte(`{"RetentionInDays":7}`), nil); p != nil {
		t.Fatalf("expected no patch when in sync, got %s", p)
	}
	p, _ = BuildPatch(live, []byte(`{"LogGroupName":"a"}`), []string{"RetentionInDays"})
	if string(p) != `[{"op":"remove","path":"/RetentionInDays"}]` {
		t.Fatalf("removable not honoured: %s", p)
	}
}

func TestBuildPatchPolicyDocumentCanonicalForm(t *testing.T) {
	live := []byte(`{"PolicyName":"p","PolicyDocument":"{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Action\":\"xray:PutTraceSegments\",\"Resource\":\"*\"}]}"}`)
	desired := []byte(`{"PolicyName":"p","PolicyDocument":"{\"Version\": \"2012-10-17\", \"Statement\": [{\"Effect\": \"Allow\", \"Action\": [\"xray:PutTraceSegments\"], \"Resource\": \"*\"}]}"}`)
	if p, err := BuildPatch(live, desired, nil); err != nil || p != nil {
		t.Fatalf("expected no drift for canonical-equivalent policy, got %s %v", p, err)
	}
}
