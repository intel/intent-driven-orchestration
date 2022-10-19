package plugins

import (
	"testing"

	protobufs "github.com/intel/intent-driven-orchestration/pkg/api/plugins/v1alpha1/protobufs"

	"github.com/stretchr/testify/assert"
)

func TestGetNextStateRequest(t *testing.T) {
	vSet := generateActuatorValidationSet()
	vSetGrpc := generateActuatorGrpcValidationSet()
	r := getNextStateRequest(vSet.start, vSet.goal, vSet.profiles)
	assert.Equal(t, vSetGrpc.start.String(), r.State.String())
	assert.Equal(t, vSetGrpc.goal.String(), r.Goal.String())
	assert.Equal(t, vSetGrpc.profiles, r.Profiles)
}

func TestGetNextStateResponse(t *testing.T) {
	var response protobufs.NextStateResponse
	vSet := generateActuatorValidationSet()
	vSetGrpc := generateActuatorGrpcValidationSet()
	response.States = vSetGrpc.end
	response.Utilities = vSetGrpc.utilities
	response.Actions = vSetGrpc.actions
	e, u, a := getNextStateResponse(&response)
	assert.Equal(t, vSet.end, e)
	assert.Equal(t, vSet.utilities, u)
	assert.Equal(t, vSet.actions, a)
}

func TestGetPerformRequest(t *testing.T) {
	vSet := generateActuatorValidationSet()
	vSetGrpc := generateActuatorGrpcValidationSet()
	r := getPerformRequest(vSet.start, vSet.actions)
	assert.Equal(t, vSetGrpc.start.String(), r.State.String())
	assert.Equal(t, vSetGrpc.actions, r.Plan)
}

func TestGetEffectRequest(t *testing.T) {
	vSet := generateActuatorValidationSet()
	vSetGrpc := generateActuatorGrpcValidationSet()
	r := getEffectRequest(vSet.start, vSet.profiles)
	assert.Equal(t, vSetGrpc.start.String(), r.State.String())
	assert.Equal(t, vSetGrpc.profiles, r.Profiles)
}
