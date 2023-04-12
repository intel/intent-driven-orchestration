package main

import (
	"testing"
)

type specCPUConfig struct {
	CPUMax                     int64
	CPURounding                int64
	CPUSafeGuardFactor         float64
	MaxProActiveCPU            int64
	ProActiveLatencyPercentage float64
}

func setCPUConfigValues(CPUMax, CPURounding, MaxProActiveCPU int64,
	CPUSafeGuardFactor, ProActiveLatencyPercentage float64) specCPUConfig {
	return specCPUConfig{
		CPUMax:                     CPUMax,
		CPURounding:                CPURounding,
		CPUSafeGuardFactor:         CPUSafeGuardFactor,
		MaxProActiveCPU:            MaxProActiveCPU,
		ProActiveLatencyPercentage: ProActiveLatencyPercentage,
	}
}

func Test_isValidConf(t *testing.T) {
	tests := []struct {
		name    string
		args    specCPUConfig
		wantErr bool
	}{
		{
			name:    "tc",
			args:    setCPUConfigValues(4000, 100, 0, 0.95, 0.1),
			wantErr: false,
		},
		{
			name:    "tc-1",
			args:    setCPUConfigValues(-10, 100, 0, 0.95, 0.1),
			wantErr: true, // negative cpu
		},
		{
			name:    "tc-2",
			args:    setCPUConfigValues(0, 100, 0, 0.95, 0.1),
			wantErr: true, // zero cpu
		},
		{
			// over uplimit
			name:    "tc-3",
			args:    setCPUConfigValues(999999999999999999, 100, 0, 0.95, 0.1),
			wantErr: true, // over limit cpu
		},
		{
			name:    "tc-4",
			args:    setCPUConfigValues(4000, -10, 0, 0.95, 0.1),
			wantErr: true, // negative round base
		},
		{
			name:    "tc-5",
			args:    setCPUConfigValues(4000, 0, 0, 0.95, 0.1),
			wantErr: true, // zero round base
		},
		{
			name:    "tc-6",
			args:    setCPUConfigValues(4000, 1001, 0, 0.95, 0.1),
			wantErr: true, // over limit round base
		},
		{
			name:    "tc-7",
			args:    setCPUConfigValues(4000, 101, 0, 0.95, 0.1),
			wantErr: true, // not round base 10
		},
		{
			name:    "tc-8",
			args:    setCPUConfigValues(4000, 100, -10, 0.95, 0.1),
			wantErr: true, // negative cpu for proactive
		},
		{
			name:    "tc-9",
			args:    setCPUConfigValues(4000, 100, 100000, 0.95, 0.1),
			wantErr: true, // over limit cpu for proactive
		},
		{
			name:    "tc-10",
			args:    setCPUConfigValues(4000, 100, 1700, -0.9, 0.1),
			wantErr: true, // negative value for safeguard
		},
		{
			name:    "tc-11",
			args:    setCPUConfigValues(4000, 100, 1700, 0, 0.1),
			wantErr: true, // zero value for safeguard
		},
		{
			name:    "tc-12",
			args:    setCPUConfigValues(4000, 100, 1700, 2.12, 0.1),
			wantErr: true, // over limit for safeguard
		},
		{
			name:    "tc-13",
			args:    setCPUConfigValues(4000, 100, 1700, 0.12, -0.21),
			wantErr: true, // negative proactive latency fraction
		},
		{
			name:    "tc-14",
			args:    setCPUConfigValues(4000, 100, 1700, 0.12, 0),
			wantErr: false, // aceptable proactive latency fraction
		},
		{
			name:    "tc-15",
			args:    setCPUConfigValues(4000, 100, 1700, 0.12, 2),
			wantErr: true, // over limit proactive latency fraction
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := isValidConf(tt.args.CPUMax, tt.args.CPURounding, tt.args.MaxProActiveCPU,
				tt.args.CPUSafeGuardFactor, tt.args.ProActiveLatencyPercentage); (err != nil) != tt.wantErr {
				t.Errorf("isValidConf() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
