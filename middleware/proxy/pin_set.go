package proxy

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
)

const (
	errMalformedPin       = "failed to decode pin %q (%s)"
	errNoPeerCertificates = "no peer certificates presented by server"
	errUnknownPin         = "pin %q not in pinset"
)

type UnknownPinsCallback func(publicKey interface{}, pin string)

type PinSet struct {
	Callback UnknownPinsCallback
	set      [][]byte
}

func NewPinSet(pins []string) (*PinSet, error) {

	set := make([][]byte, len(pins))

	for i, e := range pins {

		buf, err := base64.StdEncoding.DecodeString(e)

		if err != nil {
			return nil, fmt.Errorf(errMalformedPin, e, err)
		}

		set[i] = buf

	}

	return &PinSet{
		set: set,
	}, nil

}

func (p *PinSet) Check(c *tls.ConnectionState) error {

	certs := c.PeerCertificates

	if len(certs) < 1 {
		return errors.New(errNoPeerCertificates)
	}

	cert := certs[0]

	der, err := x509.MarshalPKIXPublicKey(cert.PublicKey)

	if err != nil {
		return err
	}

	hash := sha256.Sum256(der)

	for _, e := range p.set {

		if bytes.Equal(hash[:], e) {
			return nil
		}

	}

	pin := base64.StdEncoding.EncodeToString(hash[:])

	if p.Callback != nil {
		p.Callback(cert.PublicKey, pin)
	}

	return fmt.Errorf(errUnknownPin, pin)

}
