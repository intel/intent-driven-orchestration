package plugins

import (
	"github.com/stretchr/testify/assert"

	"testing"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner"
	"github.com/intel/intent-driven-orchestration/pkg/planner/actuators"
)

type TestMe struct {
	t    *testing.T
	vSet *ActuatorValidationSet
}

func validateActuator(t *testing.T, e []common.State, u []float64, a []planner.Action, vSet *ActuatorValidationSet) {
	assert.Equal(t, vSet.end, e)
	assert.Equal(t, vSet.utilities, u)
	assert.Equal(t, vSet.actions, a)
}

func mockedNextStateFunc(_ *common.State, _ *common.State, _ map[string]common.Profile) ([]common.State, []float64, []planner.Action) {
	vSet := generateActuatorValidationSet()
	return vSet.end, vSet.utilities, vSet.actions
}

func (tme *TestMe) mockedPerformFunc(state *common.State, plan []planner.Action) {
	assert.Equal(tme.t, *tme.vSet.start, *state)
	assert.Equal(tme.t, tme.vSet.actions, plan)
}

func (tme *TestMe) mockedEffectFunc(state *common.State, profiles map[string]common.Profile) {
	assert.Equal(tme.t, *tme.vSet.start, *state)
	assert.Equal(tme.t, tme.vSet.profiles, profiles)
}

func TestNewActuatorStub(t *testing.T) {
	a := NewActuatorPluginStub("test-actuator-1", "localhost", 3334, "localhost", 3333)
	assert.NotNil(t, a)
}

func TestNewActuatorPluginStubWithSuccessfulRegistration(t *testing.T) {
	pm := NewPluginManagerServer([]actuators.Actuator{}, "localhost", 3333)
	assert.NotNil(t, pm)
	s := NewActuatorPluginStub("test-actuator-1", "localhost", 3334, "localhost", 3333)
	assert.NotNil(t, s)

	err := pm.Start()
	assert.Nil(t, err)
	err = s.Start()
	assert.Nil(t, err)
	err = s.Register()
	assert.Nil(t, err)
	pm.mu.Lock()
	_, ok := pm.registeredPlugins["test-actuator-1"]
	pm.mu.Unlock()
	assert.True(t, ok)
	err = s.Stop()
	assert.Nil(t, err)
	err = pm.Stop()
	assert.Nil(t, err)
}

func TestNewActuatorPluginWithUnsupportedVersion(t *testing.T) {
	pm := NewPluginManagerServer([]actuators.Actuator{}, "localhost", 3333)
	assert.NotNil(t, pm)
	s := NewActuatorPluginStub("test-actuator-1", "localhost", 3334, "localhost", 3333)
	s.version = "v2"
	assert.NotNil(t, s)
	err := pm.Start()
	assert.Nil(t, err)
	err = s.Start()
	assert.Nil(t, err)
	err = s.Register()
	assert.NotNil(t, err)
	err = s.Stop()
	assert.Nil(t, err)
	pm.mu.Lock()
	_, ok := pm.registeredPlugins["test-actuator-1"]
	pm.mu.Unlock()
	assert.False(t, ok)
	err = pm.Stop()
	assert.Nil(t, err)
}

func TestPluginDeregistration(t *testing.T) {
	pm := NewPluginManagerServer([]actuators.Actuator{}, "localhost", 3333)
	assert.NotNil(t, pm)
	s := NewActuatorPluginStub("test-actuator-1", "localhost", 3334, "localhost", 3333)
	assert.NotNil(t, s)
	err := s.Start()
	assert.Nil(t, err)
	err = pm.Start()
	assert.Nil(t, err)
	err = s.Register()
	assert.Nil(t, err)
	pm.mu.Lock()
	v, ok := pm.registeredPlugins["test-actuator-1"]
	pm.mu.Unlock()
	assert.True(t, ok)
	err = s.Stop()
	v.clientConn.Close()
	assert.Nil(t, err)
	pm.refreshRegisteredPlugin(0)
	pm.mu.Lock()
	_, ok = pm.registeredPlugins["test-actuator-1"]
	pm.mu.Unlock()
	assert.False(t, ok)
	err = pm.Stop()
	assert.Nil(t, err)
}

func TestActuatorStubNextState(t *testing.T) {
	pm := NewPluginManagerServer([]actuators.Actuator{}, "localhost", 3333)
	assert.NotNil(t, pm)
	s := NewActuatorPluginStub("test-actuator-1", "localhost", 3334, "localhost", 3333)
	assert.NotNil(t, s)
	s.SetNextStateFunc(mockedNextStateFunc)
	err := pm.Start()
	assert.Nil(t, err)
	err = s.Start()
	assert.Nil(t, err)
	err = s.Register()
	assert.Nil(t, err)
	vSet := generateActuatorValidationSet()
	f := func(act actuators.Actuator) {
		e, u, a := act.NextState(vSet.start, vSet.goal, vSet.profiles)
		validateActuator(t, e, u, a, vSet)
	}
	pm.Iter(f)
	err = s.Stop()
	assert.Nil(t, err)
	err = pm.Stop()
	assert.Nil(t, err)
}

func TestActuatorPerform(t *testing.T) {
	pm := NewPluginManagerServer([]actuators.Actuator{}, "localhost", 3333)
	assert.NotNil(t, pm)
	s := NewActuatorPluginStub("test-actuator-1", "localhost", 3334, "localhost", 3333)
	assert.NotNil(t, s)
	vSet := generateActuatorValidationSet()
	tMe := &TestMe{
		t:    t,
		vSet: generateActuatorValidationSet(),
	}
	s.SetPerformFunc(tMe.mockedPerformFunc)
	err := pm.Start()
	assert.Nil(t, err)
	err = s.Start()
	assert.Nil(t, err)
	err = s.Register()
	assert.Nil(t, err)
	f := func(act actuators.Actuator) {
		act.Perform(vSet.start, vSet.actions)
	}
	pm.Iter(f)
	err = s.Stop()
	assert.Nil(t, err)
	err = pm.Stop()
	assert.Nil(t, err)
}

func TestActuatorEffect(t *testing.T) {
	pm := NewPluginManagerServer([]actuators.Actuator{}, "localhost", 3333)
	assert.NotNil(t, pm)
	s := NewActuatorPluginStub("test-actuator-1", "localhost", 3334, "localhost", 3333)
	assert.NotNil(t, s)
	vSet := generateActuatorValidationSet()
	tMe := &TestMe{
		t:    t,
		vSet: generateActuatorValidationSet(),
	}
	s.SetEffectFunc(tMe.mockedEffectFunc)
	err := pm.Start()
	assert.Nil(t, err)
	err = s.Start()
	assert.Nil(t, err)
	err = s.Register()
	assert.Nil(t, err)
	f := func(act actuators.Actuator) {
		act.Effect(vSet.start, vSet.profiles)
	}
	pm.Iter(f)
	err = s.Stop()
	assert.Nil(t, err)
	err = pm.Stop()
	assert.Nil(t, err)
}
