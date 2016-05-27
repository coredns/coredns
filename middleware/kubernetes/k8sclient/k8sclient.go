package k8sclient

import (
    "net/url"
)

// API strings
const (
    api_base   = "/api/v1"
    api_namespaces = "/namespaces"
    api_services   = "/services"
)

// Defaults
const (
    default_baseurl = "http://localhost:8080"
)


type K8sConnector struct {
    baseUrl string
}

func (self *K8sConnector) SetBaseUrl(u string) error {
    valid_url, error := url.Parse(u)

    if error != nil {
        return error
    }
    self.baseUrl = valid_url.String()

    return nil
}

func (self *K8sConnector) GetBaseUrl() string {
    return self.baseUrl
}


func (self *K8sConnector) GetResourceList() *ResourceList {
    resources := new(ResourceList)
    
    error := getJson((self.baseUrl + api_base), resources)
    if error != nil {
        return nil
    }

    return resources
}


func (self *K8sConnector) GetNamespaceList() *NamespaceList {
    namespaces := new(NamespaceList)

    error := getJson((self.baseUrl + api_base + api_namespaces), namespaces)
    if error != nil {
        return nil
    }

    return namespaces
}


func (self *K8sConnector) GetServiceList() *ServiceList {
    services := new(ServiceList)

    error := getJson((self.baseUrl + api_base + api_services), services)
    if error != nil {
        return nil
    }

    return services
}


func (self *K8sConnector) GetNamespaceNames() []string {
    /*
     * Return list of namespace names found in k8s.
     */
    var namespaces []string
    return namespaces
}


func (self *K8sConnector) NamespaceExists(name string) bool {
    /*
     * Return true if namespace exists in k8s
     */
    var exists bool
    return exists
}


func (self *K8sConnector) ServiceExists(namespace string, name string) bool {
    var exists bool
    return exists
}


func (self *K8sConnector) GetServiceNamesInNamespace(namespace string) []string {
    var names []string
    return names
}


func NewK8sConnector(baseurl string) *K8sConnector {
    k := new(K8sConnector)
    k.SetBaseUrl(baseurl)

    return k
}
