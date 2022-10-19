package common

import "testing"

// Tests for success.

// TestPodAvailabilityForSuccess tests for success.
func TestParseConfigForSuccess(t *testing.T) {
	_, err := ParseConfig("../../defaults.json")
	if err != nil {
		t.Errorf("Now this should have worked :-)")
	}
}

// Tests for failure.

// TestParseConfigForFailure tests for failure.
func TestParseConfigForFailure(t *testing.T) {
	_, err := ParseConfig("foo.yaml")
	if err == nil {
		t.Errorf("The code did not return an error!")
	}

	_, err = ParseConfig("config.go")
	if err == nil {
		t.Errorf("The code did not return an error!")
	}
}

// Tests for sanity.

// n/a

func TestCheckURL(t *testing.T) {
	type args struct {
		urlpath string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"tc-1", args{urlpath: "mongodb://planner-mongodb-service:27017/"}, true},
		{"tc-2", args{urlpath: "xjkldaoiu/"}, false},
		{"tc-3", args{urlpath: "http://prometheus-operated.monitoring:9090/api/v1/query"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkURL(tt.args.urlpath); got != tt.want {
				t.Errorf("checkURL() = %v, want %v", got, tt.want)
			}
		})
	}
}
