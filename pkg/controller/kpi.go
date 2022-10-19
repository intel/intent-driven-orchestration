package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/intel/intent-driven-orchestration/pkg/common"

	"k8s.io/klog/v2"
)

// round represents the rounding factor. // TODO: make configurable.
const round = 1000

// Client represents a http client.
var Client httpClient

// httpClient interface enables use testing with mocks.
type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// init makes sure we use the "real" http client when not testing.
func init() {
	Client = &http.Client{}
}

// getFloat returns a float64 from that weird Prometheus string.
func getFloat(unk interface{}) float64 {
	v := reflect.ValueOf(unk)
	v = reflect.Indirect(v)
	if v.Type().ConvertibleTo(reflect.TypeOf("")) {
		sv := v.Convert(reflect.TypeOf(""))
		s := sv.String()
		val, err := strconv.ParseFloat(s, 64)
		if err != nil {
			klog.Errorf("failed to parse to float: %s", err)
			return -1.0
		}
		return val
	}
	return -1.0
}

// prometheusResponse models the way prometheus API returns values.
type prometheusResponse struct {
	Data struct {
		Result []struct {
			Metric map[string]string `json:"metric"`
			Value  []interface{}     `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

// doQuery asks a Prometheus compatible endpoint for the current value.
func doQuery(profile common.Profile, objective common.Intent) float64 {
	defer func() {
		if err := recover(); err != nil {
			klog.Errorf("failed: %v - but we're recovering", err)
		}
	}()
	var query string
	if profile.External {
		query = profile.Query
	} else {
		if !strings.Contains(objective.TargetKey, "/") && strings.Count(objective.TargetKey, "/") != 1 {
			return -1.0
		}
		tmp := strings.Split(objective.TargetKey, "/")
		kind := strings.ToLower(objective.TargetKind)
		query = fmt.Sprintf(profile.Query, tmp[0], kind, tmp[1], kind)
	}

	request, err := http.NewRequest(http.MethodGet, profile.Address+"?query="+url.QueryEscape(query), nil)
	if err != nil {
		klog.Errorf("Could not construct new request: %s", err)
		return -1.0
	}
	response, err := Client.Do(request)
	if err != nil {
		klog.Errorf("Could not perform request: %s", err)
		return -1.0
	}
	if response.StatusCode == 200 {
		body, err := io.ReadAll(response.Body)
		if err != nil {
			klog.Errorf("Could not read body: %s.", err)
			return -1.0
		}
		var result prometheusResponse
		err = json.Unmarshal(body, &result)
		if err != nil {
			klog.V(1).Infof("Could not unmarshal json: %s.", err)
			return -1.0
		}
		if len(result.Data.Result) != 0 {
			val := getFloat(result.Data.Result[0].Value[1])
			if math.IsNaN(val) {
				val = -1.0
			}
			return math.Round(val*round) / round
		}
	}
	klog.Warningf("Sth went wrong while trying get information from Prometheus - will return -1.0. Status code was: %v.", response.StatusCode)
	return -1.0
}

// podAvailability calculates the availability for a single POD.
func podAvailability(podErrors []common.PodError, now time.Time) float64 {
	// Second seems to be fine for now - might check ms.
	if len(podErrors) == 0 {
		return 1.0
	}
	lastFailure := podErrors[0].Created
	var tbf []float64
	var ttr []float64
	for _, failure := range podErrors {
		tbf = append(tbf, failure.Start.Sub(lastFailure).Seconds())
		lastFailure = failure.End

		ttr = append(ttr, failure.End.Sub(failure.Start).Seconds())
	}
	tbf = append(tbf, now.Sub(lastFailure).Seconds())

	meanTimeToRepair := average(ttr)
	meanTimeBetweenFailure := average(tbf)
	return meanTimeBetweenFailure / (meanTimeBetweenFailure + meanTimeToRepair)
}

// PodSetAvailability calculates the availability for a set of PODs.
func PodSetAvailability(pods map[string]common.PodState) float64 {
	// TODO: maybe refactor this to some place else - actuators use this.
	if len(pods) < 1 {
		return 1.0
	}
	res := 1.0
	for _, v := range pods {
		res *= 1 - v.Availability
	}
	return 1.0 - res
}

// average over a list of floats.
func average(numbs []float64) float64 {
	total := 0.0
	for _, item := range numbs {
		total += item
	}
	return total / float64(len(numbs))
}
