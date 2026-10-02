package cert

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	. "testing"

	"bitbucket.org/lomoware/lomo-backend/common/cmd"
	. "gopkg.in/check.v1"
)

type certSuite struct{}

var _ = Suite(&certSuite{})

func TestCertSuite(t *T) {
	TestingT(t)
}

func (cs *certSuite) SetUpSuite(c *C)    {}
func (cs *certSuite) TearDownSuite(c *C) {}
func (cs *certSuite) SetUpTest(c *C)     {}
func (cs *certSuite) TearDownTest(c *C)  {}

func (cs *certSuite) TestCertBasic(c *C) {
	cert := &Cert{}

	c.Assert(cert.GenerateCert(false), IsNil)
	c.Assert(cert.private, NotNil)
	c.Assert(len(cert.certBytes) > 300, Equals, true, Commentf("%d", len(cert.certBytes)))

	certFile, err := ioutil.TempFile("", "cert-")
	c.Assert(err, IsNil)
	defer certFile.Close()
	defer os.Remove(certFile.Name())

	c.Assert(cert.WriteCert(certFile), IsNil)
	_, err = certFile.Seek(0, 0) // we re-use this fd later
	c.Assert(err, IsNil)

	keyFile, err := ioutil.TempFile("", "key-")
	c.Assert(err, IsNil)
	defer keyFile.Close()
	defer os.Remove(keyFile.Name())

	c.Assert(cert.WriteKey(keyFile), IsNil)
	_, err = keyFile.Seek(0, 0)
	c.Assert(err, IsNil)

	cert2, err := ReadCert(certFile, nil)
	c.Assert(err, IsNil)
	c.Assert(len(cert2.certBytes), Not(Equals), 0)
	c.Assert(cert2.certificate, NotNil)

	c.Assert(cert2.ReadKey(keyFile), IsNil)
	c.Assert(cert2.private, DeepEquals, cert.private)
}

func (cs *certSuite) TestCertCA(c *C) {
	ca := &Cert{}

	c.Assert(ca.GenerateCert(true), IsNil)
	c.Assert(ca.private, NotNil)
	c.Assert(len(ca.certBytes) > 300, Equals, true, Commentf("%d", len(ca.certBytes)))

	cert := &Cert{CA: ca}
	c.Assert(cert.GenerateCert(false), IsNil)
	c.Assert(cert.certificate.CheckSignatureFrom(ca.certificate), IsNil)

	caFile, err := ioutil.TempFile("", "cert-")
	c.Assert(err, IsNil)
	defer os.Remove(caFile.Name())
	c.Assert(ca.WriteCert(caFile), IsNil)
	caFile.Close()

	certFile, err := ioutil.TempFile("", "cert-")
	c.Assert(err, IsNil)
	defer os.Remove(caFile.Name())
	c.Assert(ca.WriteCert(certFile), IsNil)
	certFile.Close()

	out, err := cmd.Run("openssl", "verify", "-CAfile", caFile.Name(), certFile.Name())
	c.Assert(err, IsNil)
	c.Assert(strings.TrimSpace(string(out)), Equals, fmt.Sprintf("%s: OK", certFile.Name()))

	c.Assert(cert.Verify(), Equals, true)
}

func (cs *certSuite) TestFromFiles(c *C) {
	ca := &Cert{}
	c.Assert(ca.GenerateCert(true), IsNil)

	cert := &Cert{CA: ca}
	c.Assert(cert.GenerateCert(false), IsNil)

	cafile, err := ioutil.TempFile("", "ca-")
	c.Assert(err, IsNil)
	c.Assert(ca.WriteCert(cafile), IsNil)
	cafile.Close()

	certfile, err := ioutil.TempFile("", "cert-")
	c.Assert(err, IsNil)
	c.Assert(cert.WriteCert(certfile), IsNil)
	certfile.Close()

	keyfile, err := ioutil.TempFile("", "key-")
	c.Assert(err, IsNil)
	c.Assert(cert.WriteKey(keyfile), IsNil)
	keyfile.Close()

	cert, err = FromFiles(cafile.Name(), certfile.Name(), keyfile.Name())
	c.Assert(err, IsNil)

	c.Assert(cert.Verify(), Equals, true)
}
