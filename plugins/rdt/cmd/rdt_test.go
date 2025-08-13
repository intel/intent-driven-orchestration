package main

import (
	"testing"
)

// pathToAnalyticsScript defines the path to an existing analytics script for this actuator.
const pathToAnalyticsScript = "../../../pkg/planner/actuators/platform/analytics/rdt_effect.py"

// pathToPredictScript defines the path to an existing prediction script for this actuator.
const pathToPredictScript = "../../../pkg/planner/actuators/platform/analytics/rdt_predict.py"

func TestIsValidConf(t *testing.T) {
	type args struct {
		interpreter      string
		analyticsScript  string
		predictionScript string
		options          []string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "tc-0",
			args:    args{"python3", pathToAnalyticsScript, pathToPredictScript, []string{"cos0", "cos1"}},
			wantErr: false,
		},
		{
			name:    "tc-1",
			args:    args{"", pathToAnalyticsScript, pathToPredictScript, []string{"cos0", "cos1"}},
			wantErr: true, // wrong interpreter
		},
		{
			name:    "tc-2",
			args:    args{"python3", "", pathToPredictScript, []string{"cos0", "cos1"}},
			wantErr: true, // invalid analytics script
		},
		{
			name:    "tc-3",
			args:    args{"python3", pathToAnalyticsScript, "", []string{"cos0", "cos1"}},
			wantErr: true, // invalid prediction script
		},
		{
			name:    "tc-4",
			args:    args{"python3", pathToAnalyticsScript, pathToPredictScript, []string{}},
			wantErr: true, // invalid cos options
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := isValidConf(tt.args.interpreter, tt.args.analyticsScript, tt.args.predictionScript, tt.args.options); (err != nil) != tt.wantErr {
				t.Errorf("isValidConf() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
