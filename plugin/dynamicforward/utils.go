package dynamicforward

import (
	"fmt"
	"github.com/coredns/caddy"
	"time"
)

// DynamicForwardConfig хранит параметры блока
type DynamicForwardConfig struct {
	Namespace   string
	ServiceName string
	PortName    string
	Expire      time.Duration
	HealthCheck time.Duration
}

// ParseConfig parse conf CoreFile
func ParseConfig(c *caddy.Controller) (*DynamicForwardConfig, error) {
	config := &DynamicForwardConfig{
		Expire:      30 * time.Minute, // Default value
		HealthCheck: 10 * time.Second, // Default value
	}

	c.RemainingArgs()
	// Checking the presence of a parameter block
	for c.NextBlock() {
		switch c.Val() {
		case "namespace":
			if !c.NextArg() {
				return nil, c.ArgErr() // Отсутствует значение
			}
			config.Namespace = c.Val()
		case "service_name":
			if !c.NextArg() {
				return nil, c.ArgErr() // Отсутствует значение
			}
			config.ServiceName = c.Val()
		case "port_name":
			if !c.NextArg() {
				return nil, c.ArgErr() // Отсутствует значение
			}
			config.PortName = c.Val()
		case "expire":
			if !c.NextArg() {
				return nil, c.ArgErr() // Отсутствует значение
			}
			duration, err := time.ParseDuration(c.Val())
			if err != nil {
				return nil, fmt.Errorf("invalid expire duration: %v", err)
			}
			config.Expire = duration
		case "health_check":
			if !c.NextArg() {
				return nil, c.ArgErr() // Отсутствует значение
			}
			duration, err := time.ParseDuration(c.Val())
			if err != nil {
				return nil, fmt.Errorf("invalid health_check duration: %v", err)
			}
			config.HealthCheck = duration

		default:
			return nil, c.Errf("unknown parameter: %s", c.Val())
		}
	}

	// Checking the required parameters
	if config.Namespace == "" || config.ServiceName == "" || config.PortName == "" {
		return nil, fmt.Errorf("namespace, servicename, and portname are required parameters")
	}

	return config, nil
}
