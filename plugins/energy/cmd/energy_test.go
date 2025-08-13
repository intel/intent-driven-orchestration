package main

import (
	"testing"
)

// pathToAnalyticsScript defines the path to an existing script for this actuator.
const pathToAnalyticsScript = "../../../pkg/planner/actuators/energy/analytics/analytics.py"

// pathToAnalyticsScript defines the path to an existing script for this actuator.
const pathToPredictScript = "../../../pkg/planner/actuators/energy/analytics/predict.py"

func TestIsValidConf(t *testing.T) {
	tests := []struct {
		name             string
		interpreter      string
		analyticsScript  string
		predictionScript string
		steps            int
		renewableLimit   float64
		profiles         []string
		wantErr          bool
	}{
		{
			name:             "ExpectNoError",
			interpreter:      "python3",
			analyticsScript:  pathToAnalyticsScript,
			predictionScript: pathToPredictScript,
			steps:            2,
			renewableLimit:   0.9,
			profiles:         []string{"shared", "bal-pwr", "bal-perf", "perf"},
			wantErr:          false,
		},
		{
			name:             "FaultyInterpreter",
			interpreter:      "",
			analyticsScript:  pathToAnalyticsScript,
			predictionScript: pathToPredictScript,
			steps:            2,
			renewableLimit:   0.9,
			profiles:         []string{"shared", "bal-pwr", "bal-perf", "perf"},
			wantErr:          true,
		},
		{
			name:             "NoneShouldBeOkay",
			interpreter:      "python3",
			analyticsScript:  "None",
			predictionScript: "None",
			steps:            2,
			renewableLimit:   0.9,
			profiles:         []string{"shared", "bal-pwr", "bal-perf", "perf"},
			wantErr:          false,
		},
		{
			name:             "FaultyAnalyticsScript",
			interpreter:      "python3",
			analyticsScript:  "foo.py",
			predictionScript: pathToPredictScript,
			steps:            2,
			renewableLimit:   0.9,
			profiles:         []string{"shared", "bal-pwr", "bal-perf", "perf"},
			wantErr:          true,
		},
		{
			name:             "FaultyPredictionScript",
			interpreter:      "python3",
			analyticsScript:  pathToAnalyticsScript,
			predictionScript: "bar.py",
			steps:            2,
			renewableLimit:   0.9,
			profiles:         []string{"shared", "bal-pwr", "bal-perf", "perf"},
			wantErr:          true,
		},
		{
			name:             "StepDownTooSmall",
			interpreter:      "python3",
			analyticsScript:  pathToAnalyticsScript,
			predictionScript: pathToPredictScript,
			steps:            0,
			renewableLimit:   0.9,
			profiles:         []string{"shared", "bal-pwr", "bal-perf", "perf"},
			wantErr:          true,
		},
		{
			name:             "StepDownTooBig",
			interpreter:      "python3",
			analyticsScript:  pathToAnalyticsScript,
			predictionScript: pathToPredictScript,
			steps:            5,
			renewableLimit:   0.9,
			profiles:         []string{"shared", "bal-pwr", "bal-perf", "perf"},
			wantErr:          true,
		},
		{
			name:             "RenewableLimitTooSmall",
			interpreter:      "python3",
			analyticsScript:  pathToAnalyticsScript,
			predictionScript: pathToPredictScript,
			steps:            1,
			renewableLimit:   0.0,
			profiles:         []string{"shared", "bal-pwr", "bal-perf", "perf"},
			wantErr:          true,
		},
		{
			name:             "TooFewProfiles",
			interpreter:      "python3",
			analyticsScript:  pathToAnalyticsScript,
			predictionScript: pathToPredictScript,
			steps:            1,
			renewableLimit:   0.1,
			profiles:         []string{"a"},
			wantErr:          true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := isValidConf(tt.interpreter, tt.analyticsScript, tt.predictionScript, tt.steps, tt.renewableLimit, tt.profiles); (err != nil) != tt.wantErr {
				t.Errorf("isValidConf() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
