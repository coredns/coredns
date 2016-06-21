package nametemplate

import (
    "fmt"
    "strings"
)

// Likely symbols that require support:
// ${id}
// ${ip}
// ${portname}
// ${protocolname}
// ${servicename}
// ${namespace}
// ${type}              "svc" or "pod"
// ${zone}


// SkyDNS normal services have an A-record of the form "${servicename}.${namespace}.${type}.${zone}"
// This resolves to the cluster IP of the service.

// SkyDNS headless services have an A-record of the form "${servicename}.${namespace}.${type}.${zone}"
// This resolves to the set of IPs of the pods selected by the Service. Clients are expected to
// consume the set or else use round-robin selection from the set.


var symbols = map[string]string{
    "service": "${service}",
    "namespace": "${namespace}",
    "type": "${type}",
    "zone": "${zone}",
}

var types = []string{
	"svc",
	"pod",
}


// TODO: Validate that provided NameTemplate string only contains:
//			* valid, known symbols, or
//			* static strings

// TODO: Support collapsing multiple segments into a symbol. Either:
//			* all left-over segments are used as the "service" name, or
//			* some scheme like "${namespace}.${namespace}" means use
//			  segments concatenated with a "." for the namespace, or
//			* ${namespace2:4} means use segements 2->4 for the namespace.

// TODO: possibly need to store length of segmented format to handle cases
//       where query string segments to a shorter or longer list than the template.
//		 When query string segments to shorter than template:
//			* either wildcards are being used, or
//			* we are not looking up an A, AAAA, or SRV record (eg NS), or
//			* we can just short-circuit failure before hitting the k8s API.
//		 Where the query string is longer than the template, need to define which
//		 symbol consumes the other segments. Most likely this would be the servicename.
//		 Also consider how to handle static strings in the format template.
type NameTemplate struct {
    formatString string
    splitFormat []string
    // Element is a map of element name :: index in the segmented record name for the named element
    Element map[string]int
}


func (t *NameTemplate) SetTemplate(s string) error {
    var err error
	fmt.Println()

    t.Element = map[string]int{}

    t.formatString = s
    t.splitFormat = strings.Split(t.formatString, ".")
//    fmt.Println(splitFormat)
    for templateIndex, v := range t.splitFormat {
        for name, symbol := range symbols {
//            fmt.Printf("name: %v   symbol: %v:\n", name, symbol)
            if v == symbol {
                t.Element[name] = templateIndex
                break
            }
        }
    }

    return err
}


// TODO: Note this only returns the first segment in the zone. Fix this.
func (t *NameTemplate) GetZoneFromSegmentArray(segments []string) string {
	if index, ok := t.Element["zone"]; ! ok {
		return ""
	} else {
		return strings.Join(segments[index:len(segments)], ".")
	}


func (t *NameTemplate) GetNamespaceFromSegmentArray(segments []string) string {
	return t.GetSymbolFromSegmentArray("namespace", segments)
}


func (t *NameTemplate) GetServiceFromSegmentArray(segments []string) string {
	return t.GetSymbolFromSegmentArray("service", segments)
}


func (t *NameTemplate) GetTypeFromSegmentArray(segments []string) string {
	return t.GetSymbolFromSegmentArray("type", segments)
}


func (t *NameTemplate) GetSymbolFromSegmentArray(symbol string, segments []string) string {
	if index, ok := t.Element[symbol]; ! ok {
		return ""
	} else {
		return segments[index]
	}
}
