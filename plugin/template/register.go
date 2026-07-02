//go:build coredns_all || coredns_template

package template

import "github.com/coredns/coredns/plugin"

func init() { plugin.Register("template", setupTemplate) }
