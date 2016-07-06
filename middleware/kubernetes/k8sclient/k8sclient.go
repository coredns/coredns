package k8sclient

import (
	"errors"
    "fmt"
    "net/url"
    "strings"
)

// API strings
const (
    apiBase       = "/api/v1"
    apiNamespaces = "/namespaces"
    apiServices   = "/services"
)

// Defaults
const (
    defaultBaseURL = "http://localhost:8080"
)


type K8sConnector struct {
    baseURL string
}

func (c *K8sConnector) SetBaseURL(u string) error {
    url, error := url.Parse(u)

    if error != nil {
        return error
    }

	if ! url.IsAbs() {
		return errors.New("k8sclient: Kubernetes endpoint url must be an absolute URL")
	}

    c.baseURL = url.String()
    return nil
}

func (c *K8sConnector) GetBaseURL() string {
    return c.baseURL
}


// URL constructor separated from code to support dependency injection
// for unit tests.
var makeURL = func(parts []string) string {
    return strings.Join(parts, "")
}


func (c *K8sConnector) GetResourceList() *ResourceList {
    resources := new(ResourceList)

    url := makeURL([]string{c.baseURL, apiBase})
    error := parseJson(url, resources)
	// TODO: handle no response from k8s
    if error != nil {
		fmt.Printf("[ERROR] Response from kubernetes API is: %v\n", error)
        return nil
    }

    return resources
}


func (c *K8sConnector) GetNamespaceList() *NamespaceList {
    namespaces := new(NamespaceList)

    url := makeURL([]string{c.baseURL, apiBase, apiNamespaces})
    error := parseJson(url, namespaces)
    if error != nil {
        return nil
    }

    return namespaces
}


func (c *K8sConnector) GetServiceList() *ServiceList {
    services := new(ServiceList)

    url := makeURL([]string{c.baseURL, apiBase, apiServices})
    error := parseJson(url, services)
	// TODO: handle no response from k8s
    if error != nil {
		fmt.Printf("[ERROR] Response from kubernetes API is: %v\n", error)
        return nil
    }

    return services
}


// GetServicesByNamespace returns a map of
// namespacename :: [ kubernetesServiceItem ]
func (c *K8sConnector) GetServicesByNamespace() map[string][]ServiceItem {

    items := make(map[string][]ServiceItem)

    k8sServiceList := c.GetServiceList()

	// TODO: handle no response from k8s
	if k8sServiceList == nil {
		return nil
	}

    k8sItemList := k8sServiceList.Items

    for _, i := range k8sItemList {
        namespace := i.Metadata.Namespace
        items[namespace] = append(items[namespace], i)
    }

    return items
}


// GetServiceItemInNamespace returns the ServiceItem that matches
// servicename in the namespace
func (c *K8sConnector) GetServiceItemInNamespace(namespace string, servicename string) *ServiceItem {

    itemMap := c.GetServicesByNamespace()

    // TODO: Handle case where namesapce == nil

    for _, x := range itemMap[namespace] {
        if x.Metadata.Name == servicename {
            return &x
        }
    }

    // No matching item found in namespace
    return nil
}


func NewK8sConnector(baseURL string) *K8sConnector {
    k := new(K8sConnector)

	if baseURL == "" {
		baseURL = defaultBaseURL
	}

    err := k.SetBaseURL(baseURL)
	if err != nil {
		return nil
	}

    return k
}
