package dane

import (
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

func init() { plugin.Register("dane", setup) }

func setup(c *caddy.Controller) error {
	d, err := daneParse(c)
	if err != nil {
		return plugin.Error("dane", err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		d.Next = next
		return d
	})

	return nil
}

func fillRecordFromBlock(block *pem.Block, usage uint8, target []string) error {
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("unable to parse certificate: %s", err)
	}
	pk, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return fmt.Errorf("unable to serialize certificate public key: %s", err)
	}
	hash256 := sha256.Sum256(block.Bytes)
	idx, _ := certIndex(usage, 0, 1)
	target[idx] = hex.EncodeToString(hash256[:])
	hash512 := sha512.Sum512(block.Bytes)
	idx, _ = certIndex(usage, 0, 2)
	target[idx] = hex.EncodeToString(hash512[:])
	hash256 = sha256.Sum256(pk)
	idx, _ = certIndex(usage, 1, 1)
	target[idx] = hex.EncodeToString(hash256[:])
	hash512 = sha512.Sum512(pk)
	idx, _ = certIndex(usage, 1, 2)
	target[idx] = hex.EncodeToString(hash512[:])
	return nil
}

func generateRecords(fileName string) ([]string, error) {
	reader, err := os.Open(filepath.Clean(fileName))
	if err != nil {
		return nil, plugin.Error("dane", err)
	}
	defer reader.Close()
	bytes, err := io.ReadAll(reader)
	if err != nil {
		return nil, plugin.Error("dane", err)
	}
	var first *pem.Block
	var last *pem.Block
	for {
		var block *pem.Block
		block, bytes = pem.Decode(bytes)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		if first == nil {
			first = block
		}
		last = block
	}
	if first == nil {
		return nil, plugin.Error("dane", errors.New("no certificates found in file"))
	}
	records := make([]string, 8)
	err = fillRecordFromBlock(first, 1, records)
	if err != nil {
		return nil, plugin.Error("dane", err)
	}
	err = fillRecordFromBlock(last, 0, records)
	if err != nil {
		return nil, plugin.Error("dane", err)
	}
	return records, nil
}

func daneParse(c *caddy.Controller) (*Dane, error) {
	config := dnsserver.GetConfig(c)

	d := &Dane{
		Certificates: make(map[string][]string),
	}

	if c.Next() {
		d.Zones = plugin.OriginsFromArgsOrServerBlock(c.RemainingArgs(), c.ServerBlockKeys)

		for c.NextBlock() {
			switch x := c.Val(); x {
			case "file":
				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				key := c.Val()
				if !c.NextArg() {
					return nil, c.ArgErr()
				}
				fileName := c.Val()

				if !filepath.IsAbs(fileName) && config.Root != "" {
					fileName = filepath.Join(config.Root, fileName)
				}
				var err error
				d.Certificates[key], err = generateRecords(fileName)
				if err != nil {
					return nil, err
				}
			default:
				return nil, c.Errf("unknown property '%s'", x)
			}
		}
	}
	return d, nil
}
