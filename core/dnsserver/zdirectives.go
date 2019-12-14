// generated by directives_generate.go; DO NOT EDIT

package dnsserver

// Directives are registered in the order they should be
// executed.
//
// Ordering is VERY important. Every plugin will
// feel the effects of all other plugin below
// (after) them during a request, but they must not
// care what plugin above them are doing.
var Directives = []string{
	"metadata",
	"cancel",
	"tls",
	"reload",
	"nsid",
	"bufsize",
	"root",
	"bind",
	"debug",
	"trace",
	"ready",
	"health",
	"pprof",
	"prometheus",
	"errors",
	"log",
	"dnstap",
	"acl",
	"any",
	"chaos",
	"loadbalance",
	"cache",
	"rewrite",
	"dnssec",
	"autopath",
	"template",
	"transfer",
	"hosts",
	"route53",
	"azure",
	"clouddns",
	"federation",
	"k8s_external",
	"kubernetes",
	"file",
	"auto",
	"secondary",
	"etcd",
	"consul",
	"loop",
	"forward",
	"grpc",
	"erratic",
	"whoami",
	"on",
	"sign",
}
