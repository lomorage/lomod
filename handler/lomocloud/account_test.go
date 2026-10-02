package lomocloud

import (
	"net"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common/types"
	. "gopkg.in/check.v1"
)

func (ts *mainSuite) TestCreateUpdateIPMap(c *C) {
	publicIP1 := net.ParseIP("1.1.1.1")
	publicIP2 := net.ParseIP("10.10.10.10")
	reply, err := ts.h.handleGetIPMap(publicIP1)
	c.Assert(err, IsNil)
	c.Assert(len(reply), Equals, 0)
	reply, err = ts.h.handleGetIPMap(publicIP2)
	c.Assert(err, IsNil)
	c.Assert(len(reply), Equals, 0)

	// migrate case: user only register public ip, and private ip
	mac1, err := net.ParseMAC("11:22:33:44:55:66")
	c.Assert(err, IsNil)
	mac2, err := net.ParseMAC("11:22:33:44:55:77")
	c.Assert(err, IsNil)
	mac3, err := net.ParseMAC("11:22:33:44:55:88")
	c.Assert(err, IsNil)
	mac4, err := net.ParseMAC("11:22:33:44:55:99")
	c.Assert(err, IsNil)

	mac1Req := &types.IPMapRequest{PrivateIP: net.ParseIP("192.168.1.1"), MAC: mac1}
	mac2Req := &types.IPMapRequest{PrivateIP: net.ParseIP("192.168.1.2"), MAC: mac2}
	ipmap1 := map[string]*types.IPMapRequest{mac1.String(): mac1Req, mac2.String(): mac2Req}
	ipmap2 := map[string]*types.IPMapRequest{
		mac3.String(): {UUID: "uuid3", Name: "new3", Port: 1234,
			PrivateIP: net.ParseIP("192.168.1.1"), MAC: mac3},
		mac4.String(): {UUID: "uuid4", Name: "new4", Port: 4321,
			PrivateIP: net.ParseIP("192.168.1.2"), MAC: mac4},
	}
	ts.h.handleCreateOrUpdateIPMapRequest(publicIP1.String(), false, []*types.IPMapRequest{
		ipmap1[mac1.String()], ipmap1[mac2.String()],
	})

	reply, err = ts.h.handleGetIPMap(publicIP1)
	c.Assert(err, IsNil)
	c.Assert(len(reply), Equals, 2)
	for _, r := range reply {
		m, ok := ipmap1[r.MAC.String()]
		c.Assert(ok, Equals, true)
		c.Assert(r, DeepEquals, m.MkReply())
	}

	// update new data and get again
	mac1Req.UUID = "uuid1"
	mac1Req.Name = "name1"
	mac1Req.Port = 1111
	mac2Req.UUID = "uuid2"
	mac2Req.Name = "name2"
	mac2Req.Port = 2222

	ts.h.handleCreateOrUpdateIPMapRequest(publicIP1.String(), false, []*types.IPMapRequest{
		mac1Req, mac2Req,
	})

	reply, err = ts.h.handleGetIPMap(publicIP1)
	c.Assert(err, IsNil)
	c.Assert(len(reply), Equals, 2)
	for _, r := range reply {
		m, ok := ipmap1[r.MAC.String()]
		c.Assert(ok, Equals, true)
		c.Assert(r, DeepEquals, m.MkReply())
	}

	// current case: user register all data
	ts.h.handleCreateOrUpdateIPMapRequest(publicIP2.String(), false, []*types.IPMapRequest{
		ipmap2[mac3.String()], ipmap2[mac4.String()],
	})
	reply, err = ts.h.handleGetIPMap(publicIP2)
	c.Assert(err, IsNil)
	c.Assert(len(reply), Equals, 2)
	for _, r := range reply {
		m, ok := ipmap2[r.MAC.String()]
		c.Assert(ok, Equals, true)
		c.Assert(r, DeepEquals, m.MkReply())
	}

	// sleep 2 * dead timeout, and get will return empty
	time.Sleep(2 * ts.h.conf.DeadTimeout)

	reply, err = ts.h.handleGetIPMap(publicIP1)
	c.Assert(err, IsNil)
	c.Assert(len(reply), Equals, 0)
	reply, err = ts.h.handleGetIPMap(publicIP2)
	c.Assert(err, IsNil)
	c.Assert(len(reply), Equals, 0)
}
