// Package rewrite is middleware for rewriting requests internally to something different.
package rewrite

// Add more fields keywords here
var Fields = map[string]Rule{
	"qname": QnameRule{},
	"qtype": QtypeRule{},
	"class": ClassRule{},
}
