package controller

import (
	"context"
	"reflect"
	"testing"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// TestGetEffectForSanity tests for failure.
func TestGetEffectForSanity(t1 *testing.T) {
	type args struct {
		name            string
		group           string
		profileName     string
		lookBackMinutes int
		createType      func() interface{}
	}
	tests := []struct {
		name    string
		client  *mongo.Client
		args    args
		want    interface{}
		wantErr bool
	}{
		// TODO: Add positive test case.
		{name: "tc1", client: &mongo.Client{}, args: args{
			name: "mock", group: "mocklist", profileName: "profile", lookBackMinutes: 1, createType: nil}, want: nil, wantErr: true},
		{name: "tc2", client: &mongo.Client{}, args: args{
			name: "", group: "mocklist", profileName: "profile", lookBackMinutes: 1, createType: nil}, want: nil, wantErr: true},
		{name: "tc3", client: &mongo.Client{}, args: args{
			name: "", group: "", profileName: "", lookBackMinutes: 1, createType: nil}, want: nil, wantErr: true},
		{name: "tc4", client: nil, args: args{
			name: "", group: "", profileName: "/", lookBackMinutes: -100000000000, createType: createType()}, want: nil, wantErr: true},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := MongoTracer{
				client: tt.client,
			}
			got, err := t.GetEffect(tt.args.name, tt.args.group, tt.args.profileName, tt.args.lookBackMinutes, tt.args.createType)
			if (err != nil) != tt.wantErr {
				t1.Errorf("GetEffect() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t1.Errorf("GetEffect() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func createType() func() interface{} {
	return nil
}

// TestTraceEventForSanity tests for failure.
func TestTraceEventForSanity(t1 *testing.T) {
	type args struct {
		current common.State
		desired common.State
		plan    []planner.Action
	}
	tests := []struct {
		name   string
		client *mongo.Client
		args   args
	}{
		// TODO: Add test cases. NEED TO ADD POSITIVE TEST CASE
		{name: "tc1", client: &mongo.Client{}, args: args{plan: []planner.Action{}}},
		{name: "tc2", client: nil, args: args{plan: []planner.Action{}}},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := MongoTracer{
				client: tt.client,
			}
			t.TraceEvent(tt.args.current, tt.args.desired, tt.args.plan)
		})
	}
}

// TestNewMongoTracerForSanity tests for failure.
func TestNewMongoTracerForSanity(t *testing.T) {
	mt := mtest.New(t, mtest.NewOptions().ClientType(mtest.Mock))
	defer mt.Close()
	mt.Client, _ = mongo.Connect(context.TODO(), options.Client().ApplyURI(MongoURIForTesting))
	tests := []struct {
		name     string
		mongoURI string
		want     *MongoTracer
	}{
		// TODO: Add positive test cases.
		{name: "tc1", mongoURI: "other:123", want: &MongoTracer{nil}},
		{name: "tc2", mongoURI: "mongodb://bar:321", want: &MongoTracer{nil}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewMongoTracer(tt.mongoURI); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewMongoTracer() = %v, want %v", got, tt.want)
			}
		})
	}
}
