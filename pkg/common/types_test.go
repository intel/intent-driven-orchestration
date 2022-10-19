package common

import (
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

// TestProfileTypeFromTextForSanity tests for success.
func TestProfileTypeFromTextForSuccess(t *testing.T) {
	res := ProfileTypeFromText("latency")
	if res != Latency {
		t.Error("Conversion failed!")
	}

}

// TestDistanceForSuccess tests for success.
func TestDistanceForSuccess(t *testing.T) {
	s0 := State{
		Intent: Intent{
			Key:        "default/foo",
			Priority:   0,
			TargetKey:  "",
			TargetKind: "",
			Objectives: map[string]float64{
				"p99": 2,
				"p90": 0.75,
				"p50": 0.25,
			},
		},
	}
	s1 := State{
		Intent: Intent{
			Key:        "default/foo",
			Priority:   0,
			TargetKey:  "",
			TargetKind: "",
			Objectives: map[string]float64{
				"p99": 1,
				"p90": 0.5,
				"p50": 0.15,
			},
		},
	}
	profiles := map[string]Profile{
		"p99": {ProfileType: ProfileTypeFromText("latency")},
		"p95": {ProfileType: ProfileTypeFromText("latency")},
		"p50": {ProfileType: ProfileTypeFromText("latency")},
	}
	s0.Distance(&s1, profiles)
}

// TestDeepCopyStateForSuccess tests for success.
func TestDeepCopyStateForSuccess(t *testing.T) {
	state := State{
		Intent: Intent{
			Key:        "default/foo",
			Priority:   0,
			TargetKey:  "foo-deployment",
			TargetKind: "Deployment",
			Objectives: map[string]float64{},
		},
	}
	state.DeepCopy()
}

// TestCompareStateForSuccess tests for success.
func TestCompareStateForSuccess(t *testing.T) {
	s0 := State{
		Intent: Intent{
			Key:        "default/foo",
			Priority:   0,
			TargetKey:  "foo-deployment",
			TargetKind: "Deployment",
			Objectives: map[string]float64{},
		},
	}
	s2 := s0.DeepCopy()

	profiles := map[string]Profile{
		"p99": {ProfileType: ProfileTypeFromText("latency")},
	}

	s0.Compare(&s2, profiles)
}

// Tests for failure.

// N/A

// Tests for sanity.

// TestProfileTypeFromTextForSanity tests for sanity.
func TestProfileTypeFromTextForSanity(t *testing.T) {
	if ProfileTypeFromText("latency") != Latency {
		t.Error("Conversion failed!")
	}
	if ProfileTypeFromText("availability") != Availability {
		t.Error("Conversion failed!")
	}
	if ProfileTypeFromText("throughput") != Throughput {
		t.Error("Conversion failed!")
	}
	if ProfileTypeFromText("obsolete") != Obsolete {
		t.Error("Conversion failed!")
	}
	if ProfileTypeFromText("power") != Power {
		t.Error("Conversion failed!")
	}
	if ProfileTypeFromText("steadfastness") != Obsolete {
		t.Error("Conversion failed!")
	}
}

// TestDistanceForSanity tests for sanity.
func TestDistanceForSanity(t *testing.T) {
	s0 := State{
		Intent: Intent{
			Key:        "default/foo",
			Priority:   0,
			TargetKey:  "",
			TargetKind: "",
			Objectives: map[string]float64{
				"p99": 5,
				"p90": 2,
				"p50": 1,
			},
		},
	}
	s1 := State{
		Intent: Intent{
			Key:        "default/foo",
			Priority:   0,
			TargetKey:  "",
			TargetKind: "",
			Objectives: map[string]float64{
				"p99": 4,
				"p90": 3,
				"p50": 1,
			},
		},
	}
	s2 := State{
		Intent: Intent{
			Key:        "default/foo",
			Priority:   0,
			TargetKey:  "",
			TargetKind: "",
			Objectives: map[string]float64{
				"p99": 5.5,
				"p90": 2.1,
				"p50": 1.25,
			},
		},
	}
	profiles := map[string]Profile{
		"p99": {ProfileType: ProfileTypeFromText("latency")},
		"p95": {ProfileType: ProfileTypeFromText("latency")},
		"p50": {ProfileType: ProfileTypeFromText("latency")},
	}
	distance01 := s0.Distance(&s1, profiles)
	distance12 := s1.Distance(&s2, profiles)
	distance02 := s0.Distance(&s2, profiles)
	if !(distance12 < distance01) || !(distance12 < distance02) || !(distance02 < distance01) {
		t.Errorf("Something is wrong here - check distances: %f, %f, %f", distance01, distance12, distance02)
	}
}

// TestDeepCopyStateForSanity tests for sanity.
func TestDeepCopyStateForSanity(t *testing.T) {
	state := State{
		Intent: Intent{
			Key:        "default/foo",
			Priority:   0,
			TargetKey:  "foo-deployment",
			TargetKind: "Deployment",
			Objectives: map[string]float64{
				"p99": 5,
				"p95": 2,
				"p50": 1,
			},
		},
		CurrentPods: map[string]PodState{"pod_0": {
			Resources:    map[string]resource.Quantity{"cpu": resource.MustParse("100Mi")},
			Annotations:  map[string]string{"llc": "0x1"},
			Availability: 0.7,
			NodeName:     "host0",
			State:        "Running",
		},
		},
		CurrentData: map[string]map[string]float64{"cpu_value": {"host0": 20.0}},
	}
	res := state.DeepCopy()
	res.Intent.Key = "default/bar"
	res.Intent.Priority = 1.0
	res.Intent.TargetKey = "bar-replicaset"
	res.Intent.TargetKind = "ReplicaSet"
	res.Intent.Objectives["p99"] = 2.0
	res.CurrentPods["pod_1"] = PodState{Availability: 1.0}
	res.CurrentPods["pod_0"].Annotations["llc"] = "0x2"
	res.CurrentData["cpu_value"]["host0"] = 10.0

	if state.Intent.Key != "default/foo" || res.Intent.Key != "default/bar" {
		t.Errorf("Key deepcopy failed.")
	}
	if state.Intent.Priority != 0 || res.Intent.Priority != 1.0 {
		t.Errorf("Priority deepcopy failed.")
	}
	if state.Intent.TargetKey != "foo-deployment" || res.Intent.TargetKey != "bar-replicaset" {
		t.Errorf("TargetKey deepcopy failed.")
	}
	if state.Intent.TargetKind != "Deployment" || res.Intent.TargetKind != "ReplicaSet" {
		t.Errorf("TargetKind deepcopy failed.")
	}
	if state.Intent.Objectives["p99"] != 5 || state.Intent.Objectives["p95"] != 2 || res.Intent.Objectives["p99"] != 2.0 || res.Intent.Objectives["p95"] != 2 {
		t.Errorf("Intent deepcopy failed.")
	}
	if state.CurrentPods["pod_0"].Availability != 0.7 || res.CurrentPods["pod_0"].Availability != 0.7 || res.CurrentPods["pod_1"].Availability != 1.0 {
		t.Errorf("CurrentPods deepcopy failed.")
	}
	if state.CurrentPods["pod_0"].Annotations["llc"] != "0x1" || res.CurrentPods["pod_0"].Annotations["llc"] != "0x2" || res.CurrentPods["pod_1"].Availability != 1.0 {
		t.Errorf("CurrentPods deepcopy failed.")
	}
	if state.CurrentData["cpu_value"]["host0"] != 20.0 || res.CurrentData["cpu_value"]["host0"] != 10.0 {
		t.Errorf("CurrentData deepcopy failed: %v - %v", state.CurrentData, res.CurrentData)
	}

	// check if deep-copy with nils works...
	tmp0 := State{Intent: Intent{
		Key:        "test-my-objective",
		Priority:   0,
		TargetKey:  "my-deployment",
		TargetKind: "Deployment",
		Objectives: map[string]float64{"p99latency": 40},
	},
		CurrentPods: map[string]PodState{"dummy_0": {Resources: nil, Annotations: nil, Availability: 1.0, NodeName: "", State: "Running"}},
		CurrentData: map[string]map[string]float64{"cpu_value": {"host0": 20}},
	}
	tmp1 := tmp0.DeepCopy()
	if reflect.DeepEqual(tmp0, tmp1) != true {
		t.Errorf("Should be equal!")
	}
}

// TestCompareStateForSanity tests for sanity.
func TestCompareStateForSanity(t *testing.T) {
	s0 := State{
		Intent: Intent{
			Key:        "default/foo",
			Priority:   0,
			TargetKey:  "foo-deployment",
			TargetKind: "Deployment",
			Objectives: map[string]float64{
				"p99":          100,
				"availability": 0.99,
			},
		},
	}
	s1 := s0.DeepCopy()
	s2 := s0.DeepCopy()
	s2.Intent.Objectives["p99"] = 90
	s3 := s0.DeepCopy()
	s3.Intent.Objectives["p99"] = 120
	s4 := s0.DeepCopy()
	s4.Intent.Objectives["availability"] = 0.999
	s5 := s0.DeepCopy()
	s5.Intent.Objectives["availability"] = 0.96
	s6 := s0.DeepCopy()
	delete(s6.Intent.Objectives, "p99")

	profiles := map[string]Profile{
		"p99":          {ProfileType: ProfileTypeFromText("latency")},
		"availability": {ProfileType: ProfileTypeFromText("availability")},
	}

	// deep-copy should be equal.
	res := s0.Compare(&s1, profiles)
	if res != true {
		t.Errorf("Deepcopy should lead to true: %v - %v.", s0, s1)
	}
	// s2 and s4 should lead to true
	if s2.Compare(&s0, profiles) != true || s4.Compare(&s0, profiles) != true {
		t.Errorf("Should be better as s0: %v - %v.", s0, s1)
	}
	// s3 and s5 should lead to false
	if s3.Compare(&s0, profiles) != false || s5.Compare(&s0, profiles) != false {
		t.Errorf("Should be worse than s0: %v - %v.", s0, s1)
	}
	// s6 does not even have all the objectives defined.
	if s6.Compare(&s0, profiles) != false {
		t.Errorf("Should be uncomparable --> false.")
	}
}
