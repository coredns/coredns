package k8sclient

import (
    "testing"
)


var validUrls = []string{
						 "http://www.github.com",
						 "http://www.github.com:8080",
						 "http://8.8.8.8",
						 "http://8.8.8.8:9090",
						 "www.github.com:8080",
						}


var	invalidUrls = []string{
							"www.github.com",
							"8.8.8.8",
							"8.8.8.8:1010",
							"8.8`8.8",
						  }


func TestNewK8sConnector(t *testing.T) {
	var conn *K8sConnector
	var url string

	// Create with empty URL
	conn = nil
	url = ""

	conn = NewK8sConnector("")
	if conn == nil {
		t.Errorf("Expected K8sConnector instance. Instead got '%v'", conn)
	}
	url = conn.GetBaseUrl()
	if url != defaultBaseUrl {
		t.Errorf("Expected K8sConnector instance to be initialized with defaultBaseUrl. Instead got '%v'", url)
	}

	// Create with valid URL
	for _, validUrl := range validUrls {
		conn = nil
		url = ""

		conn = NewK8sConnector(validUrl)
		if conn == nil {
			t.Errorf("Expected K8sConnector instance. Instead got '%v'", conn)
		}
		url = conn.GetBaseUrl()
		if url != validUrl {
			t.Errorf("Expected K8sConnector instance to be initialized with supplied url '%v'. Instead got '%v'", validUrl, url)
		}
	}

	// Create with invalid URL
	for _, invalidUrl := range invalidUrls {
		conn = nil
		url = ""

		conn = NewK8sConnector(invalidUrl)
		if conn != nil {
			t.Errorf("Expected to not get K8sConnector instance. Instead got '%v'", conn)
			continue
		}
	}
}


func TestSetBaseUrl(t *testing.T) {
	// SetBaseUrl with valid URLs should work...
	for _, validUrl := range validUrls {
	    conn := NewK8sConnector(defaultBaseUrl)
		err := conn.SetBaseUrl(validUrl)
		if err != nil {
			t.Errorf("Expected to receive nil, instead got error '%v'", err)
			continue
		}
		url := conn.GetBaseUrl()
		if url != validUrl {
			t.Errorf("Expected to connector url to be set to value '%v', instead set to '%v'", validUrl, url)
			continue
		}
	}


	// SetBaseUrl with invalid or non absolute URLs should not change state...
	for _, invalidUrl := range invalidUrls {
		conn := NewK8sConnector(defaultBaseUrl)
		originalUrl := conn.GetBaseUrl()

		err := conn.SetBaseUrl(invalidUrl)
		if err == nil {
			t.Errorf("Expected to receive an error value, instead got nil")
		}
		url := conn.GetBaseUrl()
		if url != originalUrl {
			t.Errorf("Expected base url to not change, instead it changed to '%v'", url)
		}
	}
}
