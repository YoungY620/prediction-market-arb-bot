package judge

import "testing"

func TestParseRejectsInconsistentPassedResult(t *testing.T) {
	_, errors := parse(`{"cycle_id":"cy","passed":true,"hard_failures":[{"gate":"research","code":"bad","path":"$","message":"bad"}],"warnings":[]}`)
	if len(errors) == 0 {
		t.Fatal("expected inconsistent judge result to fail")
	}
}

func TestParseAcceptsValidFailure(t *testing.T) {
	result, errors := parse(`{"cycle_id":"cy","passed":false,"hard_failures":[{"gate":"research","code":"bad","path":"$","message":"bad"}],"warnings":[]}`)
	if len(errors) != 0 {
		t.Fatalf("unexpected errors: %#v", errors)
	}
	if result.Passed || len(result.HardFailures) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}
