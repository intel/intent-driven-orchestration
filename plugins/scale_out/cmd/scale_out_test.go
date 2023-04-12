package main

import "testing"

func Test_isValidConf(t *testing.T) {
	type args struct {
		confMaxPods                int
		confMaxProactiveScaleOut   int
		confProActiveLatencyFactor float64
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "tc-1",
			args:    args{confMaxPods: -1, confMaxProactiveScaleOut: 0, confProActiveLatencyFactor: 0},
			wantErr: true, // negative pods number
		},
		{
			name:    "tc-2",
			args:    args{confMaxPods: 0, confMaxProactiveScaleOut: 0, confProActiveLatencyFactor: 0},
			wantErr: true, // zero pods number
		},
		{
			name:    "tc-3",
			args:    args{confMaxPods: 1110, confMaxProactiveScaleOut: 0, confProActiveLatencyFactor: 0},
			wantErr: true, // over the limit pods number
		},
		{
			name:    "tc-4",
			args:    args{confMaxPods: 11, confMaxProactiveScaleOut: -1, confProActiveLatencyFactor: 0},
			wantErr: true, // negative proactive number
		},
		{
			name:    "tc-5",
			args:    args{confMaxPods: 11, confMaxProactiveScaleOut: 129, confProActiveLatencyFactor: 0},
			wantErr: true, // over the limit
		},
		{
			name:    "tc-6",
			args:    args{confMaxPods: 11, confMaxProactiveScaleOut: 9, confProActiveLatencyFactor: -1.0},
			wantErr: true, // negative factor
		},
		{
			name:    "tc-7",
			args:    args{confMaxPods: 11, confMaxProactiveScaleOut: 9, confProActiveLatencyFactor: 0.0},
			wantErr: false,
		},
		{
			name:    "tc-8",
			args:    args{confMaxPods: 11, confMaxProactiveScaleOut: 9, confProActiveLatencyFactor: 1.01},
			wantErr: true, // over the limit
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := isValidConf(tt.args.confMaxPods, tt.args.confMaxProactiveScaleOut, tt.args.confProActiveLatencyFactor); (err != nil) != tt.wantErr {
				t.Errorf("isValidConf() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
