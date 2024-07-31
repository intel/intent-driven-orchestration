package main

import "testing"

// pathToAnalyticsScript defines the path to an existing script for this actuator.
const pathToAnalyticsScript = "../../../pkg/planner/actuators/scaling/analytics/horizontal_scaling.py"

func TestIsValidConf(t *testing.T) {
	type args struct {
		interpreter                string
		script                     string
		confMaxPods                int
		confMaxProactiveScaleOut   int
		lookBack                   int
		confProActiveLatencyFactor float64
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "tc-0",
			args:    args{interpreter: "python3", script: pathToAnalyticsScript, confMaxPods: 11, confMaxProactiveScaleOut: 9, lookBack: 1000, confProActiveLatencyFactor: 0.0},
			wantErr: false,
		},
		{
			name:    "tc-1",
			args:    args{interpreter: "", script: pathToAnalyticsScript, confMaxPods: 128, confMaxProactiveScaleOut: 0, lookBack: 1000, confProActiveLatencyFactor: 0},
			wantErr: true, // missing interpreter
		},
		{
			name:    "tc-2",
			args:    args{interpreter: "python3", script: "", confMaxPods: 128, confMaxProactiveScaleOut: 0, lookBack: 1000, confProActiveLatencyFactor: 0},
			wantErr: true, // wrong script
		},
		{
			name:    "tc-3",
			args:    args{interpreter: "python3", script: pathToAnalyticsScript, confMaxPods: -1, confMaxProactiveScaleOut: 0, lookBack: 1000, confProActiveLatencyFactor: 0},
			wantErr: true, // negative pods number
		},
		{
			name:    "tc-4",
			args:    args{interpreter: "python3", script: pathToAnalyticsScript, confMaxPods: 0, confMaxProactiveScaleOut: 0, lookBack: 1000, confProActiveLatencyFactor: 0},
			wantErr: true, // zero pods number
		},
		{
			name:    "tc-5",
			args:    args{interpreter: "python3", script: pathToAnalyticsScript, confMaxPods: 99999, confMaxProactiveScaleOut: 0, lookBack: 1000, confProActiveLatencyFactor: 0},
			wantErr: true, // over the limit pods number
		},
		{
			name:    "tc-6",
			args:    args{interpreter: "python3", script: pathToAnalyticsScript, confMaxPods: -1, confMaxProactiveScaleOut: -1, lookBack: 1000, confProActiveLatencyFactor: 0},
			wantErr: true, // negative proactive number
		},
		{
			name:    "tc-7",
			args:    args{interpreter: "python3", script: pathToAnalyticsScript, confMaxPods: 128, confMaxProactiveScaleOut: 256, lookBack: 1000, confProActiveLatencyFactor: 0},
			wantErr: true, // over the limit
		},
		{
			name:    "tc-8",
			args:    args{interpreter: "python3", script: pathToAnalyticsScript, confMaxPods: 128, confMaxProactiveScaleOut: 0, lookBack: 999999, confProActiveLatencyFactor: 0},
			wantErr: true, // lookback to long
		},
		{
			name:    "tc-9",
			args:    args{interpreter: "python3", script: pathToAnalyticsScript, confMaxPods: 128, confMaxProactiveScaleOut: 0, lookBack: -1, confProActiveLatencyFactor: 0},
			wantErr: true, // lookback negative
		},
		{
			name:    "tc-10",
			args:    args{interpreter: "python3", script: pathToAnalyticsScript, confMaxPods: 128, confMaxProactiveScaleOut: 0, lookBack: 1000, confProActiveLatencyFactor: -1.0},
			wantErr: true, // negative factor
		},
		{
			name:    "tc-11",
			args:    args{interpreter: "python3", script: pathToAnalyticsScript, confMaxPods: 128, confMaxProactiveScaleOut: 0, lookBack: 1000, confProActiveLatencyFactor: 1.2},
			wantErr: true, // over the limit
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := isValidConf(tt.args.interpreter, tt.args.script, tt.args.confMaxPods, tt.args.confMaxProactiveScaleOut, tt.args.lookBack, tt.args.confProActiveLatencyFactor); (err != nil) != tt.wantErr {
				t.Errorf("isValidConf() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
