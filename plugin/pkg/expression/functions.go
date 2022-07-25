package expression

import (
	"errors"
	"net"

	"github.com/Knetic/govaluate"
)

// MakeFunctions returns a list of custom functions available to expressions
func MakeFunctions() map[string]govaluate.ExpressionFunction {
	return map[string]govaluate.ExpressionFunction{
		"incidr": func(args ...interface{}) (interface{}, error) {
			if len(args) != 2 {
				return nil, errors.New("invalid number of arguments")
			}
			ip := net.ParseIP(args[0].(string))
			if ip == nil {
				return nil, errors.New("first argument is not an IP address")
			}
			_, cidr, err := net.ParseCIDR(args[1].(string))
			if err != nil {
				return nil, err
			}
			return cidr.Contains(ip), nil
		},
	}
}
