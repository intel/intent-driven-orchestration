package controller

import (
	"bytes"
	"io"
	"net/http"
)

// MockClient enables us to mock the http requests.
type MockClient struct{}

// Do represent the mock the http request.
func (m *MockClient) Do(req *http.Request) (*http.Response, error) {
	return mockRequest(req)
}

// mockRequest replaces the http request with anon func.
var mockRequest func(req *http.Request) (*http.Response, error)

// MockResponse sets up the mock function.
func MockResponse(messageBody string, statusCode int) {
	mockRequest = func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: statusCode,
			Body:       io.NopCloser(bytes.NewReader([]byte(messageBody))),
		}, nil
	}
}
