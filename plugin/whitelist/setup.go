package whitelist

import (
	"errors"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/kubernetes"
	"github.com/mholt/caddy"
	"github.com/spf13/viper"
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

	err = k8s.InitKubeCache()
	k8s.RegisterKubeCache(c)

	whitelist.Kubernetes = k8s
	whitelist.config()

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		whitelist.Next = next
		return whitelist
	})

	return nil
}

func (whitelist whitelist) config() {

	viper.SetConfigType("json")
	fi, err := os.Lstat(whitelist.configPath)
	if err != nil {
		log.Error("can not load whitelist config")
		return
	}
	var fileName string
	if fi.Mode()&os.ModeSymlink == 1 {
		log.Info("config symlink")
		fileName, err = filepath.EvalSymlinks(whitelist.configPath)
		if err != nil {
			log.Error(err)
			return
		}
	} else {
		fileName = whitelist.configPath
	}

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
