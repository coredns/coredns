package k8sclient

import (
	"fmt"
	"net/http"
	"net/http/httptest"
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


func TestGetNamespaceList(t *testing.T) {
	// Set up a test http server
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, namespaceListJsonData)
	}))
	defer testServer.Close()

	// Overwrite URL constructor to access testServer
	makeURL = func(parts []string) string {
		return testServer.URL
    }

	expectedNamespaces := []string{"default", "demo", "test"}
	apiConn := NewK8sConnector("")
	namespaceList := apiConn.GetNamespaceList()

	if namespaceList == nil {
		t.Errorf("Expected data from GetNamespaceList(), instead got nil")
	}

	kind := namespaceList.Kind
	if kind != "NamespaceList" {
		t.Errorf("Expected data from GetNamespaceList() to have Kind='NamespaceList', instead got Kind='%v'", kind)
	}

	// Ensure correct number of namespaces found
	expectedCount := len(expectedNamespaces)
	namespaceCount := len(namespaceList.Items)
	if namespaceCount != expectedCount {
		t.Errorf("Expected '%v' namespaces from GetNamespaceList(), instead found '%v' namespaces", expectedCount, namespaceCount)
	}

	// Check that all expectedNamespaces are found in the parsed data
	for _, ns := range expectedNamespaces {
		found := false
		for _, item := range namespaceList.Items {
			if item.Metadata.Name == ns {
				found = true
				break
			}
		} 
		if ! found {
			t.Errorf("Expected '%v' namespace is not in the parsed data from GetServicesByNamespace()", ns)
		}
	}
}


func TestGetServiceList(t *testing.T) {
	// Set up a test http server
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, serviceListJsonData)
	}))
	defer testServer.Close()

	// Overwrite URL constructor to access testServer
	makeURL = func(parts []string) string {
		return testServer.URL
    }

	expectedServices := []string{"kubernetes", "mynginx", "mywebserver"}
	apiConn := NewK8sConnector("")
	serviceList := apiConn.GetServiceList()

	if serviceList == nil {
		t.Errorf("Expected data from GetServiceList(), instead got nil")
	}

	kind := serviceList.Kind
	if kind != "ServiceList" {
		t.Errorf("Expected data from GetServiceList() to have Kind='ServiceList', instead got Kind='%v'", kind)
	}

	// Ensure correct number of services found
	expectedCount := len(expectedServices)
	serviceCount := len(serviceList.Items)
	if serviceCount != expectedCount {
		t.Errorf("Expected '%v' services from GetServiceList(), instead found '%v' services", expectedCount, serviceCount)
	}

	// Check that all expectedServices are found in the parsed data
	for _, s := range expectedServices {
		found := false
		for _, item := range serviceList.Items {
			if item.Metadata.Name == s {
				found = true
				break
			}
		} 
		if ! found {
			t.Errorf("Expected '%v' service is not in the parsed data from GetServiceList()", s)
		}
	}
}


func TestGetServicesByNamespace(t *testing.T) {
	// Set up a test http server
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, serviceListJsonData)
	}))
	defer testServer.Close()

	// Overwrite URL constructor to access testServer
	makeURL = func(parts []string) string {
		return testServer.URL
    }

	expectedNamespaces := []string{"default", "demo"}
	apiConn := NewK8sConnector("")
	servicesByNamespace := apiConn.GetServicesByNamespace()

	// Ensure correct number of namespaces found
	expectedCount := len(expectedNamespaces)
	namespaceCount := len(servicesByNamespace)
	if namespaceCount != expectedCount {
		t.Errorf("Expected '%v' namespaces from GetServicesByNamespace(), instead found '%v' namespaces", expectedCount, namespaceCount)
	}

	// Check that all expectedNamespaces are found in the parsed data
	for _, ns := range expectedNamespaces {
		_, ok := servicesByNamespace[ns]
		if ! ok {
			t.Errorf("Expected '%v' namespace is not in the parsed data from GetServicesByNamespace()", ns)
		}
	}
}


const namespaceListJsonData string = 
`{
  "kind": "NamespaceList",
  "apiVersion": "v1",
  "metadata": {
    "selfLink": "/api/v1/namespaces/",
    "resourceVersion": "121279"
  },
  "items": [
    {
      "metadata": {
        "name": "default",
        "selfLink": "/api/v1/namespaces/default",
        "uid": "fb1c92d1-2f39-11e6-b9db-0800279930f6",
        "resourceVersion": "6",
        "creationTimestamp": "2016-06-10T18:34:35Z"
      },
      "spec": {
        "finalizers": [
          "kubernetes"
        ]
      },
      "status": {
        "phase": "Active"
      }
    },
    {
      "metadata": {
        "name": "demo",
        "selfLink": "/api/v1/namespaces/demo",
        "uid": "73be8ffd-2f3a-11e6-b9db-0800279930f6",
        "resourceVersion": "111",
        "creationTimestamp": "2016-06-10T18:37:57Z"
      },
      "spec": {
        "finalizers": [
          "kubernetes"
        ]
      },
      "status": {
        "phase": "Active"
      }
    },
    {
      "metadata": {
        "name": "test",
        "selfLink": "/api/v1/namespaces/test",
        "uid": "c0be05fa-3352-11e6-b9db-0800279930f6",
        "resourceVersion": "121276",
        "creationTimestamp": "2016-06-15T23:41:59Z"
      },
      "spec": {
        "finalizers": [
          "kubernetes"
        ]
      },
      "status": {
        "phase": "Active"
      }
    }
  ]
}`


const serviceListJsonData string =
`
{
  "kind": "ServiceList",
  "apiVersion": "v1",
  "metadata": {
    "selfLink": "/api/v1/services",
    "resourceVersion": "147965"
  },
  "items": [
    {
      "metadata": {
        "name": "kubernetes",
        "namespace": "default",
        "selfLink": "/api/v1/namespaces/default/services/kubernetes",
        "uid": "fb1cb0d3-2f39-11e6-b9db-0800279930f6",
        "resourceVersion": "7",
        "creationTimestamp": "2016-06-10T18:34:35Z",
        "labels": {
          "component": "apiserver",
          "provider": "kubernetes"
        }
      },
      "spec": {
        "ports": [
          {
            "name": "https",
            "protocol": "TCP",
            "port": 443,
            "targetPort": 443
          }
        ],
        "clusterIP": "10.0.0.1",
        "type": "ClusterIP",
        "sessionAffinity": "None"
      },
      "status": {
        "loadBalancer": {}
      }
    },
    {
      "metadata": {
        "name": "mynginx",
        "namespace": "demo",
        "selfLink": "/api/v1/namespaces/demo/services/mynginx",
        "uid": "93c117ac-2f3a-11e6-b9db-0800279930f6",
        "resourceVersion": "147",
        "creationTimestamp": "2016-06-10T18:38:51Z",
        "labels": {
          "run": "mynginx"
        }
      },
      "spec": {
        "ports": [
          {
            "protocol": "TCP",
            "port": 80,
            "targetPort": 80
          }
        ],
        "selector": {
          "run": "mynginx"
        },
        "clusterIP": "10.0.0.132",
        "type": "ClusterIP",
        "sessionAffinity": "None"
      },
      "status": {
        "loadBalancer": {}
      }
    },
    {
      "metadata": {
        "name": "mywebserver",
        "namespace": "demo",
        "selfLink": "/api/v1/namespaces/demo/services/mywebserver",
        "uid": "aed62187-33e5-11e6-a224-0800279930f6",
        "resourceVersion": "138185",
        "creationTimestamp": "2016-06-16T17:13:45Z",
        "labels": {
          "run": "mywebserver"
        }
      },
      "spec": {
        "ports": [
          {
            "protocol": "TCP",
            "port": 443,
            "targetPort": 443
          }
        ],
        "selector": {
          "run": "mywebserver"
        },
        "clusterIP": "10.0.0.63",
        "type": "ClusterIP",
        "sessionAffinity": "None"
      },
      "status": {
        "loadBalancer": {}
      }
    }
  ]
}
`
