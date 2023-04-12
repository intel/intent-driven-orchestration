package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/intel/intent-driven-orchestration/pkg/common"
	"github.com/intel/intent-driven-orchestration/pkg/planner"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"k8s.io/klog/v2"
)

// MongoURIForTesting enables test cases.
const MongoURIForTesting string = "mongodb://foo:123"

// Tracer allows us to trace events & hence keep a record of what the planner did.
type Tracer interface {
	// TraceEvent adds an event to the e.g. a database.
	TraceEvent(current common.State, desired common.State, plan []planner.Action)
	// GetEffect returns the data describing the effect of an action.
	GetEffect(name string, group string, profileName string, lookBackMinutes int, constructor func() interface{}) (interface{}, error)
}

// MongoTracer wraps around a MongoDB client.
type MongoTracer struct {
	client *mongo.Client
}

// NewMongoTracer initializes a new tracer.
func NewMongoTracer(mongoURI string) *MongoTracer {
	mongoOptions := options.Client().ApplyURI(mongoURI)
	client, err := mongo.Connect(context.TODO(), mongoOptions)
	if err != nil {
		klog.Errorf("Could not connect to Mongo DB: %s", err)
		return &MongoTracer{nil}
	}
	if mongoURI != MongoURIForTesting {
		if err := client.Ping(context.TODO(), readpref.Primary()); err != nil {
			klog.Errorf("Could not ping Mongo DB: %s", err)
			return &MongoTracer{nil}
		}
	}
	return &MongoTracer{client}
}

func (t MongoTracer) TraceEvent(current common.State, desired common.State, plan []planner.Action) {
	doc := bson.D{
		{Key: "name", Value: desired.Intent.Key},
		{Key: "timestamp", Value: time.Now()},
		{Key: "current_objectives", Value: current.Intent.Objectives},
		{Key: "desired_objectives", Value: desired.Intent.Objectives},
		{Key: "resources", Value: current.Resources},
		{Key: "annotations", Value: current.Annotations},
		{Key: "pods", Value: current.CurrentPods},
		{Key: "data", Value: current.CurrentData},
		{Key: "plan", Value: plan},
	}
	if t.client == nil {
		klog.Errorf("client not connected or not right client")
		return
	}
	collection := t.client.Database("intents").Collection("events")
	_, err := collection.InsertOne(context.TODO(), doc)
	if err != nil {
		klog.Errorf("Could not insert information into the database: %s.", err)
	}
}

func (t MongoTracer) GetEffect(name string, group string, profileName string, lookBackMinutes int, createType func() interface{}) (interface{}, error) {
	if t.client == nil {
		return nil, fmt.Errorf("client not connected or incorrect client")
	}
	collection := t.client.Database("intents").Collection("effects")
	lookBack := time.Now().Add(-time.Minute * time.Duration(lookBackMinutes))

	tempResult := bson.M{}
	opts := options.FindOne()
	opts.SetSort(bson.D{{Key: "_id", Value: -1}})                               // want last doc.
	opts.SetProjection(bson.D{{Key: "data", Value: 1}, {Key: "_id", Value: 0}}) // we only care about the data.
	filter := bson.D{{Key: "group", Value: group},
		{Key: "name", Value: name},
		{Key: "profileName", Value: profileName},
		{Key: "$or", Value: bson.A{ // either this a static doc or not very old,
			bson.M{"static": true},
			bson.M{"timestamp": bson.M{"$gt": lookBack}}},
		},
	}
	err := collection.FindOne(context.TODO(), filter, opts).Decode(tempResult)
	if len(tempResult) == 0 {
		klog.V(2).Info("no effects collection created")
	}
	if err != nil {
		klog.Errorf("Error to decode: %s", err)
		return nil, err
	}
	obj, err := json.Marshal(tempResult["data"])
	if err != nil {
		klog.Errorf("Error to marshal: %s", err)
	}
	data := createType()
	err = json.Unmarshal(obj, data)
	if err != nil {
		klog.Errorf("Error unmarshal: %s ", err)
	}
	return data, err
}
