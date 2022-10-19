package controller

import (
	"testing"
	"time"

	"github.com/intel/intent-driven-orchestration/pkg/common"
)

// init makes sure we mock the http requests.
func init() {
	Client = &MockClient{}
}

// Tests for success.

// TestDoQueryForSuccess tests for success.
func TestDoQueryForSuccess(t *testing.T) {
	responseBody := "{\"data\": {\"result\": [{\"value\": [1645019125.000, 1.23456780]}]}}"
	MockResponse(responseBody, 200)
	prof := common.Profile{
		Key:         "default/my-p99-compliance",
		ProfileType: 0,
		Query:       "histogram_quantile(0.99,sum(irate(my_p99_bucket_metric[30s]))",
		External:    true,
		Address:     "127.0.0.1",
	}
	objective := common.Intent{
		Key:        "default/my-objective",
		Priority:   0,
		TargetKey:  "default/my-deployment",
		TargetKind: "Deployment",
		Objectives: nil,
	}
	doQuery(prof, objective)
}

// TestPodAvailabilityForSuccess tests for success.
func TestPodAvailabilityForSuccess(t *testing.T) {
	var errors []common.PodError
	podAvailability(errors, time.Now())
}

// TestPodSetAvailabilityForSuccess tests for success.
func TestPodSetAvailabilityForSuccess(t *testing.T) {
	PodSetAvailability(map[string]common.PodState{"pod0": {Availability: 0.8}})
}

// Tests for failure.

// TestDoQueryForFailure tests for failure.
func TestDoQueryForFailure(t *testing.T) {
	responseBody := "{}"
	MockResponse(responseBody, 500)
	prof := common.Profile{
		Key:         "default/p99latency",
		ProfileType: 0,
		Query:       "histogram_quantile(0.99,sum(irate(response_latency_ms_bucket{{namespace=\"%s\",%s=\"%s\",direction=\"inbound\"}}[30s]))by(le,%s))",
		External:    false,
		Address:     "127.0.0.1",
	}
	objective := common.Intent{
		Key:        "default/my-objective",
		Priority:   0,
		TargetKey:  "default/my-deployment",
		TargetKind: "Deployment",
		Objectives: nil,
	}
	res := doQuery(prof, objective)
	if res != -1.0 {
		t.Errorf("Should have returned -1.0 - actually returned: %f", res)
	}
}

// Tests for sanity.

// TestDoQueryForSanity tests for sanity.
func TestDoQueryForSanity(t *testing.T) {
	responseBody := "{\"data\": {\"result\": [{\"value\": [1645019125.000, \"1.23456780\"]}]}}"
	MockResponse(responseBody, 200)
	prof := common.Profile{
		Key:         "default/p99latency",
		ProfileType: 0,
		Query:       "histogram_quantile(0.99,sum(irate(response_latency_ms_bucket{{namespace=\"%s\",%s=\"%s\",direction=\"inbound\"}}[30s]))by(le,%s))",
		External:    false,
		Address:     "127.0.0.1",
	}
	objective := common.Intent{
		Key:        "default/my-objective",
		Priority:   0,
		TargetKey:  "default/my-deployment",
		TargetKind: "Deployment",
		Objectives: nil,
	}
	res := doQuery(prof, objective)
	if res != 1.235 {
		t.Errorf("Expected 1.235 - actually got: %f", res)
	}

	// json parse will fail.
	nonsense := "foo[{9]}"
	MockResponse(nonsense, 200)
	res = doQuery(prof, objective)
	if res != -1.0 {
		t.Errorf("Expected -1.0 - actually got: %f", res)
	}

	// no data
	noData := "{\"data\": {\"result\": []}}"
	MockResponse(noData, 200)
	res = doQuery(prof, objective)
	if res != -1.0 {
		t.Errorf("Expected -1.0 - actually got: %f", res)
	}

	// not a number data
	notANumber := "{\"data\": {\"result\": [{\"value\": [1645019125.000, \"NaN\"]}]}}"
	MockResponse(notANumber, 200)
	res = doQuery(prof, objective)
	if res != -1.0 {
		t.Errorf("Expected -1.0 - actually got: %f", res)
	}
}

// TestPodAvailabilityForSanity tests for sanity.
func TestPodAvailabilityForSanity(t *testing.T) {
	created, _ := time.Parse(time.RFC3339, "2022-02-16T10:00:00Z")
	start, _ := time.Parse(time.RFC3339, "2022-02-16T11:00:00Z")
	end, _ := time.Parse(time.RFC3339, "2022-02-16T11:00:30Z")
	now, _ := time.Parse(time.RFC3339, "2022-02-16T14:00:00Z")
	errors := []common.PodError{{Key: "pod_0", Start: start, End: end, Created: created}}
	res := podAvailability(errors, now)
	if res < 0.995 {
		t.Errorf("Availability for this POD should be ~ 0.995 - it was: %f.", res)
	}

	// pod_1 is not in the map.
	podErrors := map[string][]common.PodError{"pod_0": errors}
	res = podAvailability(podErrors["pod_1"], now)
	if res != 1.0 {
		t.Errorf("Availability for this POD should be 1.0 - it was: %f.", res)
	}
}

// TestPodSetAvailabilityForSanity tests for sanity.
func TestPodSetAvailabilityForSanity(t *testing.T) {
	res := PodSetAvailability(map[string]common.PodState{"pod0": {Availability: 0.8}, "pod1": {Availability: 0.8}})
	if res != 0.96 {
		t.Errorf("Expected 0.96 - got: %f", res)
	}
	res = PodSetAvailability(map[string]common.PodState{"pod0": {Availability: 0.7}, "pod1": {Availability: 0.7}, "pod2": {Availability: 0.7}, "pod3": {Availability: 0.7}})
	if res != 0.9919 {
		t.Errorf("Expected 0.9919 - got: %f", res)
	}
	res = PodSetAvailability(map[string]common.PodState{"pod0": {Availability: 0.9}, "pod1": {Availability: 0.98}})
	if res != 0.998 {
		t.Errorf("Expected 0.998 - got: %f", res)
	}
	res = PodSetAvailability(map[string]common.PodState{})
	if res != 1.0 {
		t.Errorf("Expected 1.0 - got: %f", res)
	}
}
