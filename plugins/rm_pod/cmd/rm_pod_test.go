package main

import (
	"testing"
)

func TestIsValidConf(t *testing.T) {
	type args struct {
		minPods  int
		lookBack int
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "tc-0",
			args:    args{1, 1000},
			wantErr: false,
		},
		{
			name:    "tc-1",
			args:    args{0, 1000},
			wantErr: true, // min pods to small.
		},
		{
			name:    "tc-2",
			args:    args{1024, 1000},
			wantErr: true, // min pods to large.
		},
		{
			name:    "tc-3",
			args:    args{1, -1},
			wantErr: true, // lookback negative.
		},
		{
			name:    "tc-4",
			args:    args{1, 9999999},
			wantErr: true, // lookback to large.
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := isValidConf(tt.args.minPods, tt.args.lookBack); (err != nil) != tt.wantErr {
				t.Errorf("isValidConf() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
