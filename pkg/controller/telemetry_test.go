package controller

import (
	"testing"
)

// init makes sure we mock the http requests.
func init() {
	Client = &MockClient{}
}

// Tests for success.

// TestGetHostTelemetryForSuccess tests for success.
func TestGetHostTelemetryForSuccess(_ *testing.T) {
	responseBody := "{\"data\": {\"result\": [{\"metric\": {\"exported_instance\": \"node0\"}, \"value\": [1645019125.000, 25.0]}]}}"
	MockResponse(responseBody, 200)
	query := "avg(collectd_cpu_percent{exported_instance=~\"%s\"})by(exported_instance)"
	hosts := []string{"node0, node1"}
	getHostTelemetry("127.0.0.1", query, hosts, "exported_instance")
}

// Tests for failure.

// TestGetHostTelemetryForFailure tests for failure.
func TestGetHostTelemetryForFailure(t *testing.T) {
	responseBody := "{\"data\": {\"result\": [{\"metric\": {\"exported_instance\": \"node0\"}, \"value\": [1645019125.000, 25.0]}]}}"
	MockResponse(responseBody, 500)
	query := "avg(collectd_cpu_percent{exported_instance=~\"%s\"})by(exported_instance)"
	hosts := []string{"node0, node1"}
	res := getHostTelemetry("127.0.0.1", query, hosts, "exported_instance")
	if len(res) != 0 {
		t.Errorf("Expected empty result set - got %v", res)
	}

	// json parse will fail.
	nonsense := "foo[{9]}"
	MockResponse(nonsense, 200)
	res = getHostTelemetry("127.0.0.1", query, hosts, "exported_instance")
	if len(res) != 0 {
		t.Errorf("Expected empty result set - got %v", res)
	}
}

// Tests for sanity.

// TestGetHostTelemetryForSuccess tests for sanity.
func TestGetHostTelemetryForSanity(t *testing.T) {
	responseBody := "{\"data\": {\"result\": [" +
		"{\"metric\": {\"exported_instance\": \"node0\"}, \"value\": [1645019125.000, \"25.0\"]}," +
		"{\"metric\": {\"exported_instance\": \"node1\"}, \"value\": [1645019125.000, \"12.5\"]}" +
		"]}}"
	MockResponse(responseBody, 200)
	query := "avg(collectd_cpu_percent{exported_instance=~\"%s\"})by(exported_instance)"
	hosts := []string{"node0, node1"}
	res := getHostTelemetry("127.0.0.1", query, hosts, "exported_instance")
	if host, ok := res["node0"]; !ok {
		t.Errorf("Missing node0 in result map.")
	} else {
		if host != 25.0 {
			t.Errorf("Expected 25 - got, %f", host)
		}
	}
	if host, ok := res["node1"]; !ok {
		t.Errorf("Missing node1 in result map.")
	} else {
		if host != 12.5 {
			t.Errorf("Expected 25 - got, %f", host)
		}
	}
}
