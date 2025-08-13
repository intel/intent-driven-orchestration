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
		boostFactor                float64
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
			name:    "should_work",
			args:    args{"python3", pathToAnalyticsScript, 4000, 100, 0, 1.0, 0.95, 0.1, 10000},
			wantErr: false,
		},
		{
			name:    "interpreter_empty",
			args:    args{"", pathToAnalyticsScript, 4000, 100, 0, 1.0, 0.95, 0.1, 10000},
			wantErr: true,
		},
		{
			name:    "script_empty",
			args:    args{"python3", "", 4000, 100, 0, 1.0, 0.95, 0.1, 10000},
			wantErr: true,
		},
		{
			name:    "negative_cpu",
			args:    args{"python3", pathToAnalyticsScript, -1, 100, 0, 1.0, 0.95, 0.1, 10000},
			wantErr: true,
		},
		{
			name:    "zero_cpu",
			args:    args{"python3", pathToAnalyticsScript, 0, 100, 0, 1.0, 0.95, 0.1, 10000},
			wantErr: true,
		},
		{
			name:    "over_cpu_limit",
			args:    args{"python3", pathToAnalyticsScript, 999999999, 100, 0, 1.0, 0.95, 0.1, 10000},
			wantErr: true,
		},
		{
			name:    "negative_rounding",
			args:    args{"python3", pathToAnalyticsScript, 4000, -1, 0, 1.0, 0.95, 0.1, 10000},
			wantErr: true,
		},
		{
			name:    "zero_rounding",
			args:    args{"python3", pathToAnalyticsScript, 4000, 0, 0, 1.0, 0.95, 0.1, 10000},
			wantErr: true,
		},
		{
			name:    "over_limit_rounding",
			args:    args{"python3", pathToAnalyticsScript, 4000, 1001, 0, 1.0, 0.95, 0.1, 10000},
			wantErr: true,
		},
		{
			name:    "rounding_not_base_10",
			args:    args{"python3", pathToAnalyticsScript, 4000, 101, 0, 1.0, 0.95, 0.1, 10000},
			wantErr: true,
		},
		{
			name:    "negative_proactive",
			args:    args{"python3", pathToAnalyticsScript, 4000, 100, -1, 1.0, 0.95, 0.1, 10000},
			wantErr: true,
		},
		{
			name:    "over_limit_proactive",
			args:    args{"python3", pathToAnalyticsScript, 4000, 100, 10000, 1.0, 0.95, 0.1, 10000},
			wantErr: true,
		},
		{
			name:    "boost_factor_to_small",
			args:    args{"python3", pathToAnalyticsScript, 4000, 100, 0, -1.0, 0.95, 0.1, 10000},
			wantErr: true,
		},
		{
			name:    "boost_factor_to_big",
			args:    args{"python3", pathToAnalyticsScript, 4000, 100, 0, 12.0, 0.95, 0.1, 10000},
			wantErr: true,
		},
		{
			name:    "negative_value_for_safeguard",
			args:    args{"python3", pathToAnalyticsScript, 4000, 100, 0, 1.0, -1.0, 0.1, 10000},
			wantErr: true,
		},
		{
			name:    "zero_limit_safeguard",
			args:    args{"python3", pathToAnalyticsScript, 4000, 100, 0, 1.0, 0.0, 0.1, 10000},
			wantErr: true,
		},
		{
			name:    "over_limit_safeguard",
			args:    args{"python3", pathToAnalyticsScript, 4000, 100, 0, 1.0, 2.0, 0.1, 10000},
			wantErr: true,
		},
		{
			name:    "negative_proactive_latency",
			args:    args{"python3", pathToAnalyticsScript, 4000, 100, 0, 1.0, 0.95, -1.0, 10000},
			wantErr: true,
		},
		{
			name:    "over_limit_proactive_latency",
			args:    args{"python3", pathToAnalyticsScript, 4000, 100, 0, 1.0, 0.95, 1.01, 10000},
			wantErr: true,
		},
		{
			name:    "negative_lookback",
			args:    args{"python3", pathToAnalyticsScript, 4000, 100, 0, 1.0, 0.95, 1.0, -1},
			wantErr: true,
		},
		{
			name:    "over_limit_lookback",
			args:    args{"python3", pathToAnalyticsScript, 4000, 100, 0, 1.0, 0.95, 1.0, 999999},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := isValidConf(tt.args.interpreter, tt.args.script, tt.args.cpuMax, tt.args.cpuRounding, tt.args.maxProActiveCPU,
				tt.args.boostFactor, tt.args.cpuSafeGuardFactor, tt.args.proActiveLatencyPercentage, tt.args.lookBack); (err != nil) != tt.wantErr {
				t.Errorf("isValidConf() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
