package whoareyou

import (
	"errors"
	"fmt"

	"github.com/miekg/coredns/middleware/whoareyou/ipscope"
)

func getScopes() (*ipscope.IPScopes, error) {
	var scopes *ipscope.IPScopes
	ipset, err := ipscope.InterfaceAddrs()
	if err != nil {
		return nil, errors.New(fmt.Sprint("Can't get a list of the system's network interface addresses: ", err.Error()))
	}
	scopes = ipscope.NewIPScopes(ipset)
	return scopes, nil
}
