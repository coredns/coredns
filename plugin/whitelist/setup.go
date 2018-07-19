package whitelist

import (
	"errors"
	"fmt"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/kubernetes"
	"github.com/mholt/caddy"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func init() {
	caddy.RegisterPlugin("whitelist", caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

func kubernetesParse(c *caddy.Controller) (*kubernetes.Kubernetes, error) {
	var (
		k8s *kubernetes.Kubernetes
		err error
	)

	i := 0
	for c.Next() {
		if i > 0 {
			return nil, plugin.ErrOnce
		}
		i++

		k8s, err = kubernetes.ParseStanza(c)
		if err != nil {
			return k8s, err
		}
	}
	return k8s, nil
}

func setup(c *caddy.Controller) error {

	whitelist := &whitelist{}
	if whitelistConf := os.Getenv("TUFIN_WHITELIST_CONF_FILE_JSON"); whitelistConf != "" {
		whitelist.configPath = whitelistConf
		WatchFile(whitelistConf, time.Second, whitelist.config)
	} else {
		return errors.New("please set TUFIN_WHITELIST_CONF_FILE_JSON")
	}

	k8s, err := kubernetesParse(c)
	if err != nil {
		return plugin.Error("whitelist", err)
	}

	if len(k8s.Zones) != 1 {
		return errors.New("whitelist zones length should be 1 (cluster zone only)")
	}

	err = k8s.InitKubeCache()
	k8s.RegisterKubeCache(c)

	whitelist.Kubernetes = k8s
	whitelist.config()

	time.Sleep(time.Second * 5)
	if discoveryURL := os.Getenv("TUFIN_DISCOVERY_URL"); discoveryURL != "" {
		discoveryURL, err := url.Parse(discoveryURL)
		if err == nil {
			ip := whitelist.getIpByServiceName(discoveryURL.Scheme)
			log.Infof("discovery ip %s", ip)
			dc, conn := newDiscoveryClient(fmt.Sprintf("%s:%s", ip, discoveryURL.Opaque))
			whitelist.Discovery = dc
			c.OnShutdown(func() error {
				return conn.Close()
			})
		} else {
			log.Warningf("can not parse TUFIN_DISCOVERY_URL. error %v", err)
		}
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		whitelist.Next = next
		return whitelist
	})

	return nil
}

func newDiscoveryClient(discoveryURL string) (DiscoveryServiceClient, *grpc.ClientConn) {

	cc, err := grpc.Dial(discoveryURL, grpc.WithInsecure(),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{Time: 10 * time.Minute, Timeout: 30 * time.Second, PermitWithoutStream: true}))
	if err != nil {
		log.Errorf("failed to create gRPC connection with '%v'", err)
		return nil, nil
	}

	return NewDiscoveryServiceClient(cc), cc

}

func (whitelist *whitelist) config() {

	viper.SetConfigType("json")
	fileName := whitelist.configPath
	viper.SetConfigName(strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName)))
	viper.AddConfigPath(filepath.Dir(fileName))
	viper.ReadInConfig()
	conf := viper.GetStringMapStringSlice("services")
	whitelist.ServicesToWhitelist = convert(conf)
	log.Infof("whitelist configuration %s", whitelist.ServicesToWhitelist)
}

func convert(conf map[string][]string) map[string]map[string]struct{} {

	ret := make(map[string]map[string]struct{})
	for k, v := range conf {
		ret[k] = make(map[string]struct{})
		for _, item := range v {
			ret[k][item] = struct{}{}
		}
	}

	return ret
}
