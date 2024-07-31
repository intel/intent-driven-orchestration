package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

// init makes sure we use the "real" http client when not testing.
func init() {
	Client = &http.Client{
		Timeout: 5 * time.Second,
	}
}

// getHostTelemetry returns (optional) information for a set of hosts.
func getHostTelemetry(endpoint string, query string, hosts []string, hostLabel string) map[string]float64 {
	ret := map[string]float64{}
	queryString := fmt.Sprintf(query, strings.Join(hosts, "|"))
	request, err := http.NewRequest(http.MethodGet, endpoint+"?query="+url.QueryEscape(queryString), nil)
	if err != nil {
		klog.Errorf("Could not construct request.")
		return ret
	}
	response, err := Client.Do(request)
	if err != nil {
		klog.Errorf("Could not perform request.")
		return ret
	}
	if response.StatusCode == 200 {
		body, err := io.ReadAll(response.Body)
		if err != nil {
			klog.Errorf("Could not read body: %s.", err)
			return ret
		}
		var result prometheusResponse
		err = json.Unmarshal(body, &result)
		if err != nil {
			klog.V(1).Infof("Could not unmarshal json: %s - %s.", err, body)
			return ret
		}
		for _, res := range result.Data.Result {
			host := res.Metric[hostLabel]
			val := res.Value[1]
			ret[host] = getFloat(val)
		}
	}
	return ret
}
