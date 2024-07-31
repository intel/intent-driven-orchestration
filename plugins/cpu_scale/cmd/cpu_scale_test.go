package main

import (
	"testing"
)

// pathToAnalyticsScript defines the path to an existing script for this actuator.
const pathToAnalyticsScript = "../../../pkg/planner/actuators/scaling/analytics/cpu_rightsizing.py"

func TestIsValidConf(t *testing.T) {
	type args struct {
		interpreter                string
		script                     string
		cpuMax                     int64
		cpuRounding                int64
		maxProActiveCPU            int64
		cpuSafeGuardFactor         float64
		proActiveLatencyPercentage float64
		lookBack                   int
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "tc-0",
			args:    args{"python3", pathToAnalyticsScript, 4000, 100, 0, 0.95, 0.1, 10000},
			wantErr: false,
		},
		{
			name:    "tc-1",
			args:    args{"", pathToAnalyticsScript, 4000, 100, 0, 0.95, 0.1, 10000},
			wantErr: true,
		},
		{
			name:    "tc-2",
			args:    args{"python3", "", 4000, 100, 0, 0.95, 0.1, 10000},
			wantErr: true,
		},
		{
			name:    "tc-3",
			args:    args{"python3", pathToAnalyticsScript, -1, 100, 0, 0.95, 0.1, 10000},
			wantErr: true, // negative cpu
		},
		{
			name:    "tc-4",
			args:    args{"python3", pathToAnalyticsScript, 0, 100, 0, 0.95, 0.1, 10000},
			wantErr: true, // zero cpu
		},
		{
			name:    "tc-5",
			args:    args{"python3", pathToAnalyticsScript, 999999999, 100, 0, 0.95, 0.1, 10000},
			wantErr: true, // over limit cpu
		},
		{
			name:    "tc-6",
			args:    args{"python3", pathToAnalyticsScript, 4000, -1, 0, 0.95, 0.1, 10000},
			wantErr: true, // negative round base
		},
		{
			name:    "tc-7",
			args:    args{"python3", pathToAnalyticsScript, 4000, 0, 0, 0.95, 0.1, 10000},
			wantErr: true, // zero round base
		},
		{
			name:    "tc-8",
			args:    args{"python3", pathToAnalyticsScript, 4000, 1001, 0, 0.95, 0.1, 10000},
			wantErr: true, // over limit round base
		},
		{
			name:    "tc-9",
			args:    args{"python3", pathToAnalyticsScript, 4000, 101, 0, 0.95, 0.1, 10000},
			wantErr: true, // not round base 10
		},
		{
			name:    "tc-10",
			args:    args{"python3", pathToAnalyticsScript, 4000, 100, -1, 0.95, 0.1, 10000},
			wantErr: true, // negative cpu for proactive
		},
		{
			name:    "tc-11",
			args:    args{"python3", pathToAnalyticsScript, 4000, 100, 10000, 0.95, 0.1, 10000},
			wantErr: true, // over limit cpu for proactive
		},
		{
			name:    "tc-12",
			args:    args{"python3", pathToAnalyticsScript, 4000, 100, 0, -1.0, 0.1, 10000},
			wantErr: true, // negative value for safeguard
		},
		{
			name:    "tc-13",
			args:    args{"python3", pathToAnalyticsScript, 4000, 100, 0, 0.0, 0.1, 10000},
			wantErr: true, // zero value for safeguard
		},
		{
			name:    "tc-14",
			args:    args{"python3", pathToAnalyticsScript, 4000, 100, 0, 2.0, 0.1, 10000},
			wantErr: true, // over limit for safeguard
		},
		{
			name:    "tc-15",
			args:    args{"python3", pathToAnalyticsScript, 4000, 100, 0, 0.95, -1.0, 10000},
			wantErr: true, // negative proactive latency fraction
		},
		{
			name:    "tc-16",
			args:    args{"python3", pathToAnalyticsScript, 4000, 100, 0, 0.95, 1.01, 10000},
			wantErr: true, // over limit proactive latency fraction
		},
		{
			name:    "tc-17",
			args:    args{"python3", pathToAnalyticsScript, 4000, 100, 0, 0.95, 1.0, -1},
			wantErr: true, // negative lookback.
		},
		{
			name:    "tc-18",
			args:    args{"python3", pathToAnalyticsScript, 4000, 100, 0, 0.95, 1.0, 999999},
			wantErr: true, // over limit lookback.
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := isValidConf(tt.args.interpreter, tt.args.script, tt.args.cpuMax, tt.args.cpuRounding, tt.args.maxProActiveCPU,
				tt.args.cpuSafeGuardFactor, tt.args.proActiveLatencyPercentage, tt.args.lookBack); (err != nil) != tt.wantErr {
				t.Errorf("isValidConf() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
