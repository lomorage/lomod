package cert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"io/ioutil"
	"math/big"
	"net"
	"os"
	"time"

	"github.com/pkg/errors"
)

var defaultOrganization = pkix.Name{Organization: []string{"lomoware"}}

// Cert is the structure which encapsulates an x509 certificate with ed25519
// keys.
type Cert struct {
	MaxSerial   int64
	CA          *Cert
	DNSNames    []string
	IPAddresses []net.IP

	private     *ecdsa.PrivateKey
	public      ecdsa.PublicKey
	certificate *x509.Certificate

	certBytes []byte
}

// Verify returns true if the cert can be validated against its CA.
func (c *Cert) Verify() bool {
	return c.certificate.CheckSignatureFrom(c.CA.certificate) == nil
}

// Certificate returns the *x509.Certificate associated with this Cert.
func (c *Cert) Certificate() *x509.Certificate {
	return c.certificate
}

// PrivateKey returns the private key
func (c *Cert) PrivateKey() *ecdsa.PrivateKey {
	return c.private
}

// GenerateCert generates a new cert, overwriting any stored key in the
// process. If CA is supplied it signs it with the CA. If isCA is true, the
// cert is set up as a CA.
func (c *Cert) GenerateCert(isCA bool) error {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}

	c.public = priv.PublicKey
	c.private = priv

	if c.MaxSerial == 0 {
		c.MaxSerial = 1024
	}

	serialNumber, err := rand.Int(rand.Reader, big.NewInt(c.MaxSerial))
	if err != nil {
		return err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,

		Subject: defaultOrganization,
		Issuer:  defaultOrganization,

		NotBefore: time.Now().AddDate(0, -1, 0),
		NotAfter:  time.Now().Add(time.Hour * 24 * 365 * 10),

		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},

		BasicConstraintsValid: true,

		DNSNames:    c.DNSNames,
		IPAddresses: c.IPAddresses,
	}

	if isCA {
		template.IsCA = true
		template.KeyUsage |= x509.KeyUsageCertSign
	}

	if c.CA == nil {
		c.certBytes, err = x509.CreateCertificate(rand.Reader, &template, &template, &c.public, c.private)
		if err != nil {
			return err
		}
	} else {
		csr, err := c.createCSR()
		if err != nil {
			return err
		}

		template.Signature = csr.Signature
		template.SignatureAlgorithm = csr.SignatureAlgorithm
		template.PublicKeyAlgorithm = csr.PublicKeyAlgorithm
		template.PublicKey = csr.PublicKey

		c.certBytes, err = x509.CreateCertificate(rand.Reader, &template, c.CA.certificate, &c.public, c.CA.private)
		if err != nil {
			return err
		}
	}

	c.certificate, err = x509.ParseCertificate(c.certBytes)
	return err
}

// WriteCert writes a certificate to disk at the filename supplied.
func (c *Cert) WriteCert(writer io.Writer) error {
	return pem.Encode(writer, &pem.Block{Type: "CERTIFICATE", Bytes: c.certBytes})
}

// WriteKey writes the private key for the cert
func (c *Cert) WriteKey(writer io.Writer) error {
	bytes, err := x509.MarshalECPrivateKey(c.private)
	if err != nil {
		return err
	}
	return pem.Encode(writer, &pem.Block{Type: "EC PRIVATE KEY", Bytes: bytes})
}

// FromFiles loads the filenames provided into a *Cert.
func FromFiles(cacert, certfile, keyfile string) (*Cert, error) {
	f, err := os.Open(cacert)
	if err != nil {
		return nil, err
	}

	ca, err := ReadCert(f, nil)
	f.Close()
	if err != nil {
		return nil, errors.Wrap(err, cacert)
	}

	f, err = os.Open(certfile)
	if err != nil {
		return nil, errors.Wrap(err, certfile)
	}

	cert, err := ReadCert(f, ca)
	f.Close()
	if err != nil {
		return nil, errors.Wrap(err, certfile)
	}

	f, err = os.Open(keyfile)
	if err != nil {
		return nil, errors.Wrap(err, keyfile)
	}

	err = cert.ReadKey(f)
	f.Close()

	if err != nil {
		return nil, errors.Wrap(err, keyfile)
	}

	return cert, nil
}

// ReadCert reads from a reader and returns a *Cert. Note that the private key
// must still be read for most *Cert operations to be useful.
func ReadCert(reader io.Reader, ca *Cert) (*Cert, error) {
	content, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	return ParseCert(content, ca)
}

// ParseCert parses cert from given array
func ParseCert(content []byte, ca *Cert) (*Cert, error) {
	block, _ := pem.Decode(content)
	if block == nil {
		return nil, errors.New("decode Cert PEM failed")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}

	return &Cert{
		CA:          ca,
		certBytes:   block.Bytes,
		certificate: cert,
	}, nil
}

// ReadKey reads the private key in from the reader. The public key is computed from that.
func (c *Cert) ReadKey(reader io.Reader) error {
	content, err := ioutil.ReadAll(reader)
	if err != nil {
		return err
	}
	return c.ParseKey(content)
}

// ParseKey parses the array to get private key
func (c *Cert) ParseKey(content []byte) error {
	block, _ := pem.Decode(content)
	if block == nil {
		return errors.New("decode Key PEM failed")
	}

	var err error
	c.private, err = x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return err
	}

	c.public = *c.private.Public().(*ecdsa.PublicKey)

	return nil
}

func (c *Cert) createCSR() (*x509.CertificateRequest, error) {
	csr := &x509.CertificateRequest{
		Subject:     defaultOrganization,
		IPAddresses: c.IPAddresses,
		DNSNames:    c.DNSNames,
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, csr, c.private)
	if err != nil {
		return nil, err
	}

	csr, err = x509.ParseCertificateRequest(csrBytes)
	if err != nil {
		return nil, err
	}

	return csr, csr.CheckSignature()
}
