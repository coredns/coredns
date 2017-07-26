package proxy

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"sync"
)

const errRandomnessGeneration = "failed to generate random bytes: %s"

// Padding implements fixed-size, url-safe padding of strings
type Padding struct {
	Random  io.Reader
	size    int
	buffers sync.Pool
}

// NewPadding instantiates a new padding up to the given size (e.g. 256)
func NewPadding(size uint16) *Padding {

	// calculate the buffer we for a base64 representation that is "size" long
	s := int(3.0 / 4.0 * float64(size))

	return &Padding{
		Random: rand.Reader,
		size:   s,
		buffers: sync.Pool{
			New: func() interface{} {
				return bytes.NewBuffer(make([]byte, s))
			},
		},
	}

}

// Generate produces an appropriatep adding string for the given name
func (p *Padding) Generate(name string) (string, error) {

	const empty = ""

	padding := p.size - len(name)

	if padding < 1 {
		return empty, nil
	}

	buf := p.buffers.Get().(*bytes.Buffer)
	defer buf.Reset()

	if _, err := io.ReadFull(p.Random, buf.Bytes()); err != nil {
		return empty, fmt.Errorf(errRandomnessGeneration, err)
	}

	return base64.RawURLEncoding.EncodeToString(buf.Bytes()[:padding]), nil

}
