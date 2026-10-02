package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/group"
	"bitbucket.org/lomoware/lomo-backend/common/types"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	. "gopkg.in/check.v1"
)

var shareTime = types.LomoTime{Time: time.Now()}

func (ts *mainSuite) checkShare(c *C, url string, expect *types.Records) {
	r, err := ts.request(url, http.MethodGet, 200, nil, nil)
	c.Assert(err, IsNil)
	defer r.Close()

	reply := &types.Records{}
	json.NewDecoder(r).Decode(reply)
	for _, r := range reply.Records {
		r.ShareTime = shareTime
	}
	c.Assert(reply, DeepEquals, expect)
}

func (ts *mainSuite) prepareShare(c *C) string {
	ts.h.previewDims = nil

	category, err := loadCategory()
	c.Assert(err, IsNil)

	ts.createAssets(c, &category, ts.tokenBob)

	// create the 3rd user
	u := user.User{
		Name:     "charlie",
		Password: "charlie123",
		Phone:    "charlie_phone",
		Email:    "charlie_email",
		NickName: "charlie_nick",
		HomeDir:  photodir + "/charlie",
	}
	err = ts.createUser(u, ts.token, true)
	c.Assert(err, IsNil)
	tokenCharlie, err := ts.login(u.Name, u.Password)
	c.Assert(err, IsNil)
	c.Assert(tokenCharlie, Not(Equals), "")

	// create 2 groups
	ts.requestWithMethod(c, "/group/group1?token="+ts.token, "POST", http.StatusOK, nil, nil, nil)
	ts.requestWithMethod(c, "/group/group2?token="+ts.token, "POST", http.StatusOK, nil, nil, nil)

	return tokenCharlie
}

// Refer api/test/lomod for preview/transcode asset validation.
func (ts *mainSuite) TestShareBob(c *C) {
	tokenCharlie := ts.prepareShare(c)

	ts.requestWithMethod(c, "/send/user/2/1?token="+ts.token, "POST", http.StatusOK, nil, nil, nil)
	ts.requestWithMethod(c, "/send/user/2/7.png?token="+ts.token, "POST", http.StatusOK, nil, nil, nil)
	ts.requestWithMethod(c, "/send/user/2/5d14e3ce82d2101b3b8ece22487d81d824a5745a?byhash=1&token="+ts.token, "POST", http.StatusOK, nil, nil, nil)

	myRecv := &types.ReceiveRecords{Users: []int{}, Groups: []int{}}
	myShare := &types.Records{Records: []*types.Record{}}
	// charlie should unable to find share from anyone
	ts.requestGet(c, "/receive?token="+tokenCharlie, &types.ReceiveRecords{}, myRecv)
	ts.requestGet(c, "/receive?byassets=1&token="+tokenCharlie, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/1?token="+tokenCharlie, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/1?includeme=1&token="+tokenCharlie, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/2?token="+tokenCharlie, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/2?includeme=1&token="+tokenCharlie, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/3?token="+tokenCharlie, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/3?includeme=1&token="+tokenCharlie, &types.Records{}, myShare)

	// alice should not receive any shared
	ts.requestGet(c, "/receive?token="+ts.token, &types.ReceiveRecords{}, myRecv)
	ts.requestGet(c, "/receive?byassets=1&token="+ts.token, &types.Records{}, myShare)

	// alice should not receive any from herself
	ts.requestGet(c, "/receive/user/1?token="+ts.token, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/1?includeme=1&token="+ts.token, &types.Records{}, myShare)

	// alice should share anything to charlie
	ts.requestGet(c, "/receive/user/3?token="+ts.token, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/3?includeme=1&token="+ts.token, &types.Records{}, myShare)

	// alice should share 1,2,3 to bob
	ts.requestGet(c, "/receive/user/2?token="+ts.token, &types.Records{}, myShare)
	aliceShare := &types.Records{Records: []*types.Record{
		{ID: 3, Type: 0, SenderID: 1, ReceiverID: 2, AssetID: "6.mp4", ShareTime: shareTime},
		{ID: 2, Type: 0, SenderID: 1, ReceiverID: 2, AssetID: "7.png", ShareTime: shareTime},
		{ID: 1, Type: 0, SenderID: 1, ReceiverID: 2, AssetID: "1.jpg", ShareTime: shareTime},
	}}
	ts.checkShare(c, "/receive/user/2?includeme=1&token="+ts.token, aliceShare)

	// list what shared to bob
	myRecv.Users = []int{1}
	ts.requestGet(c, "/receive?token="+ts.tokenBob, &types.ReceiveRecords{}, myRecv)
	ts.checkShare(c, "/receive?byassets=1&token="+ts.tokenBob, aliceShare)
	ts.checkShare(c, "/receive/user/2?token="+ts.tokenBob, myShare)

	ts.checkShare(c, "/receive/user/1?token="+ts.tokenBob, aliceShare)
	ts.checkShare(c, "/receive/user/1?includeme=1&token="+ts.tokenBob, aliceShare)

	// bob should share nothing to charlie
	ts.requestGet(c, "/receive/user/3?token="+ts.tokenBob, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/3?includeme=1&token="+ts.tokenBob, &types.Records{}, myShare)

	// share non exist photo should return fail
	reply := common.ErrResponse{ID: "20", Text: common.ErrAssetNotExistForUser.Error()}
	ts.requestWithMethod(c, "/send/user/2/1000?token="+ts.token, "POST", http.StatusNotFound, &common.ErrResponse{}, &reply, nil)

	// unable to share bob's photo
	ts.requestWithMethod(c, "/send/user/2/2?token="+ts.token, "POST", http.StatusNotFound, &common.ErrResponse{}, &reply, nil)

	// download asset
	ts.assetGet(c, fmt.Sprintf("/receive/asset/2?token=%s", ts.tokenBob), "921196cc106f070668dbb2ff3eafc01017540a52")

	// unable to download unexist share preview
	reply.ID = "22"
	reply.Text = common.ErrNotExistAsset.Error()
	ts.requestWithMethod(c, "/receive/preview/1000?token="+ts.tokenBob, "GET", http.StatusNotFound, &common.ErrResponse{}, &reply, nil)

	// unable to download unexist share asset
	ts.requestWithMethod(c, "/receive/asset/1000?token="+ts.tokenBob, "GET", http.StatusNotFound, &common.ErrResponse{}, &reply, nil)

	// charlie can not download bob's share
	reply.ID = "19"
	reply.Text = common.ErrAssetNotSharedToUser.Error()
	ts.requestWithMethod(c, "/receive/preview/1?token="+tokenCharlie, "GET", http.StatusNotFound, &common.ErrResponse{}, &reply, nil)
	ts.requestWithMethod(c, "/receive/asset/1?token="+tokenCharlie, "GET", http.StatusNotFound, &common.ErrResponse{}, &reply, nil)

	// delete share
	ts.requestWithMethod(c, "/send/3?token="+ts.token, "DELETE", http.StatusOK, nil, nil, nil)

	// delete it again should return failure
	reply.ID = "1"
	reply.Text = "[while find share ID: 3: sql: no rows in result set]: " + common.ErrBadRequest.Error()
	ts.requestWithMethod(c, "/send/3?token="+ts.token, "DELETE", http.StatusBadRequest, &common.ErrResponse{}, &reply, nil)

	// bob should not receive it anymore
	aliceShare.Records = aliceShare.Records[1:3]
	ts.requestGet(c, "/receive?token="+ts.tokenBob, &types.ReceiveRecords{}, myRecv)
	ts.checkShare(c, "/receive?byassets=1&token="+ts.tokenBob, aliceShare)
	ts.checkShare(c, "/receive/user/2?token="+ts.tokenBob, myShare)

	ts.checkShare(c, "/receive/user/1?token="+ts.tokenBob, aliceShare)
	ts.checkShare(c, "/receive/user/1?includeme=1&token="+ts.tokenBob, aliceShare)

	// hide share
	ts.requestWithMethod(c, "/receive/2?token="+ts.tokenBob, "DELETE", http.StatusOK, nil, nil, nil)
	aliceShare.Records = aliceShare.Records[1:2]
	ts.requestGet(c, "/receive?token="+ts.tokenBob, &types.ReceiveRecords{}, myRecv)
	ts.checkShare(c, "/receive?byassets=1&token="+ts.tokenBob, aliceShare)
	ts.checkShare(c, "/receive/user/2?token="+ts.tokenBob, myShare)

	ts.checkShare(c, "/receive/user/1?token="+ts.tokenBob, aliceShare)
	ts.checkShare(c, "/receive/user/1?includeme=1&token="+ts.tokenBob, aliceShare)

	// delete asset should also delete share
	ts.requestDelete(c, ts.token, &types.DeleteAssetItems{
		List: []types.DeleteAssetItem{{ID: "1"}},
	})

	// TODO: if alice's share is disabled, should we return
	// myRecv.Users = []int{}
	aliceShare.Records = []*types.Record{}
	ts.requestGet(c, "/receive?token="+ts.tokenBob, &types.ReceiveRecords{}, myRecv)
	ts.checkShare(c, "/receive?byassets=1&token="+ts.tokenBob, aliceShare)
	ts.checkShare(c, "/receive/user/2?token="+ts.tokenBob, myShare)

	ts.checkShare(c, "/receive/user/1?token="+ts.tokenBob, aliceShare)
	ts.checkShare(c, "/receive/user/1?includeme=1&token="+ts.tokenBob, aliceShare)
}

func (ts *mainSuite) TestShareGroup(c *C) {
	tokenCharlie := ts.prepareShare(c)

	// add bob to group 1, and charlie to group 2
	ts.requestWithMethod(c, "/group/1/2?token="+ts.token, "POST", http.StatusOK, nil, nil, nil)
	ts.requestWithMethod(c, "/group/2/3?token="+ts.token, "POST", http.StatusOK, nil, nil, nil)

	gs := group.Groups{Groups: []group.Group{
		{ID: 1, OwnerID: 1, Name: "group1", Members: []*user.User{{ID: 1, Name: "alice"}, {ID: 2, Name: "bob"}}},
		{ID: 2, OwnerID: 1, Name: "group2", Members: []*user.User{{ID: 1, Name: "alice"}, {ID: 3, Name: "charlie"}}},
	}}
	ts.requestGet(c, "/group?token="+ts.token, &group.Groups{}, &gs)

	// share the 1st asset to group 1
	ts.requestWithMethod(c, "/send/group/1/1?token="+ts.token, "POST", http.StatusOK, nil, nil, nil)

	myRecv := &types.ReceiveRecords{Users: []int{}, Groups: []int{}}
	myShare := &types.Records{Records: []*types.Record{}}
	// charlie should not find share from anyone
	ts.requestGet(c, "/receive?token="+tokenCharlie, &types.ReceiveRecords{}, myRecv)
	ts.requestGet(c, "/receive?byassets=1&token="+tokenCharlie, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/1?token="+tokenCharlie, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/1?includeme=1&token="+tokenCharlie, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/2?token="+tokenCharlie, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/2?includeme=1&token="+tokenCharlie, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/3?token="+tokenCharlie, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/3?includeme=1&token="+tokenCharlie, &types.Records{}, myShare)

	// charlie should not find share in group 1 because it is not in the group 1
	reply := common.ErrResponse{ID: "13", Text: common.ErrNotInGroup.Error()}
	ts.requestWithMethod(c, "/receive/group/1?token="+tokenCharlie, "GET", http.StatusBadRequest, &common.ErrResponse{}, &reply, nil)
	ts.requestWithMethod(c, "/receive/group/1?includeme=1&token="+tokenCharlie, "GET", http.StatusBadRequest, &common.ErrResponse{}, &reply, nil)
	// charlie should not find share in group 2
	ts.requestGet(c, "/receive/group/2?token="+tokenCharlie, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/group/2?includeme=1&token="+tokenCharlie, &types.Records{}, myShare)

	// alice should not receive any shared
	ts.requestGet(c, "/receive?token="+ts.token, &types.ReceiveRecords{}, myRecv)
	ts.requestGet(c, "/receive?byassets=1&token="+ts.token, &types.Records{}, myShare)

	// alice should receive nothing using user api
	ts.requestGet(c, "/receive/user/1?token="+ts.token, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/1?includeme=1&token="+ts.token, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/2?token="+ts.token, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/2?includeme=1&token="+ts.token, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/3?token="+ts.token, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/4?includeme=1&token="+ts.token, &types.Records{}, myShare)

	// alice should receive only if includeme
	ts.requestGet(c, "/receive/group/2?token="+ts.token, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/group/2?includeme=1&token="+ts.token, &types.Records{}, myShare)

	// alice should receive shared asset from group 1
	aliceShare := &types.Records{Records: []*types.Record{
		{ID: 1, Type: 1, SenderID: 1, ReceiverID: 1, AssetID: "1.jpg", ShareTime: shareTime},
	}}
	ts.requestGet(c, "/receive/group/1?token="+ts.token, &types.Records{}, myShare)
	ts.checkShare(c, "/receive/group/1?includeme=1&token="+ts.token, aliceShare)

	// bob should not receive anothing from user API
	ts.checkShare(c, "/receive/user/1?token="+ts.tokenBob, myShare)
	ts.checkShare(c, "/receive/user/1?includeme=1&token="+ts.tokenBob, myShare)
	ts.checkShare(c, "/receive/user/2?token="+ts.tokenBob, myShare)
	ts.checkShare(c, "/receive/user/2?includeme=1&token="+ts.tokenBob, myShare)
	ts.checkShare(c, "/receive/user/3?token="+ts.tokenBob, myShare)
	ts.checkShare(c, "/receive/user/4?includeme=1&token="+ts.tokenBob, myShare)

	// bot should be able to receive from default api and group api
	myRecv.Groups = []int{1}
	ts.requestGet(c, "/receive?token="+ts.tokenBob, &types.ReceiveRecords{}, myRecv)
	ts.checkShare(c, "/receive?byassets=1&token="+ts.tokenBob, aliceShare)
	ts.checkShare(c, "/receive/group/1?token="+ts.tokenBob, aliceShare)
	ts.checkShare(c, "/receive/group/1?includeme=1&token="+ts.tokenBob, aliceShare)

	// share non exist photo should return fail
	reply = common.ErrResponse{ID: "20", Text: common.ErrAssetNotExistForUser.Error()}
	ts.requestWithMethod(c, "/send/group/2/1000?token="+ts.token, "POST", http.StatusNotFound, &common.ErrResponse{}, &reply, nil)

	// unable to share bob's photo
	ts.requestWithMethod(c, "/send/group/2/2?token="+ts.token, "POST", http.StatusNotFound, &common.ErrResponse{}, &reply, nil)

	// unable to share non exist group
	reply.ID = "1"
	reply.Text = "[while checking group exist: 100: sql: no rows in result set]: " + common.ErrBadRequest.Error()
	ts.requestWithMethod(c, "/send/group/100/1?token="+ts.token, "POST", http.StatusBadRequest, &common.ErrResponse{}, &reply, nil)

	// download asset
	ts.assetGet(c, fmt.Sprintf("/receive/asset/1?token=%s", ts.tokenBob), "4ebf54db04f335ff66bfc1fd982be62bf23fc967")

	// unable to download unexist share preview
	reply.ID = "22"
	reply.Text = common.ErrNotExistAsset.Error()
	ts.requestWithMethod(c, "/receive/preview/1000?token="+ts.tokenBob, "GET", http.StatusNotFound, &common.ErrResponse{}, &reply, nil)

	// unable to download unexist share asset
	ts.requestWithMethod(c, "/receive/asset/1000?token="+ts.tokenBob, "GET", http.StatusNotFound, &common.ErrResponse{}, &reply, nil)

	// charlie can not download bob's share
	reply.ID = "13"
	reply.Text = common.ErrNotInGroup.Error()
	ts.requestWithMethod(c, "/receive/preview/1?token="+tokenCharlie, "GET", http.StatusBadRequest, &common.ErrResponse{}, &reply, nil)
	ts.requestWithMethod(c, "/receive/asset/1?token="+tokenCharlie, "GET", http.StatusBadRequest, &common.ErrResponse{}, &reply, nil)

	// now add charlie into group 1 and charlie should be able to receive previous share in the group, but not in user
	ts.requestWithMethod(c, "/group/1/3?token="+ts.token, "POST", http.StatusOK, nil, nil, nil)

	ts.requestGet(c, "/receive/user/1?token="+tokenCharlie, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/1?includeme=1&token="+tokenCharlie, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/2?token="+tokenCharlie, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/2?includeme=1&token="+tokenCharlie, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/3?token="+tokenCharlie, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/3?includeme=1&token="+tokenCharlie, &types.Records{}, myShare)

	ts.requestGet(c, "/receive?token="+tokenCharlie, &types.ReceiveRecords{}, myRecv)
	ts.checkShare(c, "/receive?byassets=1&token="+tokenCharlie, aliceShare)
	ts.checkShare(c, "/receive/group/1?token="+tokenCharlie, aliceShare)
	ts.checkShare(c, "/receive/group/1?includeme=1&token="+tokenCharlie, aliceShare)

	ts.assetGet(c, fmt.Sprintf("/receive/asset/1?token=%s", tokenCharlie), "4ebf54db04f335ff66bfc1fd982be62bf23fc967")

	// share the 2nd photo
	ts.requestWithMethod(c, "/send/group/1/921196cc106f070668dbb2ff3eafc01017540a52?byhash=1&token="+ts.token, "POST", http.StatusOK, nil, nil, nil)

	// download asset
	ts.assetGet(c, fmt.Sprintf("/receive/asset/2?token=%s", ts.tokenBob), "921196cc106f070668dbb2ff3eafc01017540a52")
	ts.assetGet(c, fmt.Sprintf("/receive/asset/2?token=%s", tokenCharlie), "921196cc106f070668dbb2ff3eafc01017540a52")

	aliceShare.Records = []*types.Record{
		{ID: 2, Type: 1, SenderID: 1, ReceiverID: 1, AssetID: "7.png", ShareTime: shareTime},
		{ID: 1, Type: 1, SenderID: 1, ReceiverID: 1, AssetID: "1.jpg", ShareTime: shareTime},
	}

	ts.checkShare(c, "/receive/group/1?includeme=1&token="+ts.token, aliceShare)
	ts.requestGet(c, "/receive?token="+ts.tokenBob, &types.ReceiveRecords{}, myRecv)
	ts.checkShare(c, "/receive?byassets=1&token="+ts.tokenBob, aliceShare)
	ts.checkShare(c, "/receive/group/1?token="+ts.tokenBob, aliceShare)
	ts.checkShare(c, "/receive/group/1?includeme=1&token="+ts.tokenBob, aliceShare)
	ts.requestGet(c, "/receive?token="+tokenCharlie, &types.ReceiveRecords{}, myRecv)
	ts.checkShare(c, "/receive?byassets=1&token="+tokenCharlie, aliceShare)
	ts.checkShare(c, "/receive/group/1?token="+tokenCharlie, aliceShare)
	ts.checkShare(c, "/receive/group/1?includeme=1&token="+tokenCharlie, aliceShare)

	// delete share
	ts.requestWithMethod(c, "/send/2?token="+ts.token, "DELETE", http.StatusOK, nil, nil, nil)

	// delete it again should return failure
	reply.ID = "1"
	reply.Text = "[while find share ID: 2: sql: no rows in result set]: " + common.ErrBadRequest.Error()
	ts.requestWithMethod(c, "/send/2?token="+ts.token, "DELETE", http.StatusBadRequest, &common.ErrResponse{}, &reply, nil)

	// bob should not receive it anymore
	aliceShare.Records = aliceShare.Records[1:2]
	ts.requestGet(c, "/receive?token="+ts.tokenBob, &types.ReceiveRecords{}, myRecv)
	ts.checkShare(c, "/receive?byassets=1&token="+ts.tokenBob, aliceShare)
	ts.checkShare(c, "/receive/group/1?token="+ts.tokenBob, aliceShare)
	ts.checkShare(c, "/receive/group/1?includeme=1&token="+ts.tokenBob, aliceShare)
	ts.checkShare(c, "/receive/group/1?token="+tokenCharlie, aliceShare)
	ts.checkShare(c, "/receive/group/1?includeme=1&token="+tokenCharlie, aliceShare)

	// delete asset should also delete share
	ts.requestDelete(c, ts.token, &types.DeleteAssetItems{
		List: []types.DeleteAssetItem{{ID: "1"}},
	})

	myRecv.Groups = []int{}
	aliceShare.Records = []*types.Record{}
	ts.requestGet(c, "/receive?token="+ts.tokenBob, &types.ReceiveRecords{}, myRecv)
	ts.checkShare(c, "/receive?byassets=1&token="+ts.tokenBob, aliceShare)
	ts.checkShare(c, "/receive/group/1?token="+ts.tokenBob, aliceShare)
	ts.checkShare(c, "/receive/group/1?includeme=1&token="+ts.tokenBob, aliceShare)
	ts.checkShare(c, "/receive/group/1?token="+tokenCharlie, aliceShare)
	ts.checkShare(c, "/receive/group/1?includeme=1&token="+tokenCharlie, aliceShare)
}

func (ts *mainSuite) TestShareMultiple(c *C) {
	tokenCharlie := ts.prepareShare(c)

	// add bob to group 1, and charlie to group 2
	ts.requestWithMethod(c, "/group/1/2?token="+ts.token, "POST", http.StatusOK, nil, nil, nil)
	ts.requestWithMethod(c, "/group/1/3?token="+ts.token, "POST", http.StatusOK, nil, nil, nil)

	h := types.Hash
	myShare := &types.Records{Records: []*types.Record{
		{Type: 0, ReceiverID: 2, AssetID: "1.jpg"},
		{Type: 1, ReceiverID: 1, AssetID: "7.png"},
		{Type: 1, ReceiverID: 1, AssetIDType: &h, AssetID: "5d14e3ce82d2101b3b8ece22487d81d824a5745a"},
	}}
	// share the 1st asset to group 1
	content, err := json.Marshal(myShare)
	c.Assert(err, IsNil)
	ts.requestWithMethodBody(c, "/send?token="+ts.token, "POST", http.StatusOK, bytes.NewBuffer(content), nil, nil, nil)

	myRecv := &types.ReceiveRecords{Users: []int{}, Groups: []int{}}
	myShare.Records = []*types.Record{}
	bobRecvUser := &types.Records{Records: []*types.Record{
		{ID: 1, Type: 0, SenderID: 1, ReceiverID: 2, AssetID: "1.jpg", ShareTime: shareTime},
	}}
	recvGroup := &types.Records{Records: []*types.Record{
		{ID: 3, Type: 1, SenderID: 1, ReceiverID: 1, AssetID: "6.mp4", ShareTime: shareTime},
		{ID: 2, Type: 1, SenderID: 1, ReceiverID: 1, AssetID: "7.png", ShareTime: shareTime},
	}}

	// alice receive status
	ts.requestGet(c, "/receive?token="+ts.token, &types.ReceiveRecords{}, myRecv)
	ts.requestGet(c, "/receive?byassets=1&token="+ts.token, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/1?token="+ts.token, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/1?includeme=1&token="+ts.token, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/2?token="+ts.token, &types.Records{}, myShare)
	ts.checkShare(c, "/receive/user/2?includeme=1&token="+ts.token, bobRecvUser)
	ts.requestGet(c, "/receive/user/3?token="+ts.token, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/3?includeme=1&token="+ts.token, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/group/1?token="+ts.token, &types.Records{}, myShare)
	ts.checkShare(c, "/receive/group/1?includeme=1&token="+ts.token, recvGroup)
	ts.requestGet(c, "/receive/group/2?token="+ts.token, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/group/2?includeme=1&token="+ts.token, &types.Records{}, myShare)

	// bob
	myRecv.Users = []int{1}
	myRecv.Groups = []int{1}
	aliceShare := &types.Records{Records: []*types.Record{
		{ID: 3, Type: 1, SenderID: 1, ReceiverID: 1, AssetID: "6.mp4", ShareTime: shareTime},
		{ID: 2, Type: 1, SenderID: 1, ReceiverID: 1, AssetID: "7.png", ShareTime: shareTime},
		{ID: 1, Type: 0, SenderID: 1, ReceiverID: 2, AssetID: "1.jpg", ShareTime: shareTime},
	}}
	ts.requestGet(c, "/receive?token="+ts.tokenBob, &types.ReceiveRecords{}, myRecv)
	ts.checkShare(c, "/receive?byassets=1&token="+ts.tokenBob, aliceShare)
	ts.checkShare(c, "/receive/user/1?token="+ts.tokenBob, bobRecvUser)
	ts.checkShare(c, "/receive/user/1?includeme=1&token="+ts.tokenBob, bobRecvUser)
	ts.requestGet(c, "/receive/user/2?token="+ts.tokenBob, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/2?includeme=1&token="+ts.tokenBob, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/3?token="+ts.tokenBob, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/4?includeme=1&token="+ts.tokenBob, &types.Records{}, myShare)
	ts.checkShare(c, "/receive/group/1?token="+ts.tokenBob, recvGroup)
	ts.checkShare(c, "/receive/group/1?includeme=1&token="+ts.tokenBob, recvGroup)

	reply := common.ErrResponse{ID: "13", Text: common.ErrNotInGroup.Error()}
	ts.requestWithMethod(c, "/receive/group/2?token="+ts.tokenBob, "GET", http.StatusBadRequest, &common.ErrResponse{}, &reply, nil)
	ts.requestWithMethod(c, "/receive/group/2?includeme=1&token="+ts.tokenBob, "GET", http.StatusBadRequest, &common.ErrResponse{}, &reply, nil)

	// charlie
	myRecv.Users = []int{}
	ts.requestGet(c, "/receive?token="+tokenCharlie, &types.ReceiveRecords{}, myRecv)
	ts.checkShare(c, "/receive?byassets=1&token="+tokenCharlie, recvGroup)
	ts.requestGet(c, "/receive/user/1?token="+tokenCharlie, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/1?includeme=1&token="+tokenCharlie, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/2?token="+tokenCharlie, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/2?includeme=1&token="+tokenCharlie, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/3?token="+tokenCharlie, &types.Records{}, myShare)
	ts.requestGet(c, "/receive/user/4?includeme=1&token="+tokenCharlie, &types.Records{}, myShare)
	ts.checkShare(c, "/receive/group/1?token="+tokenCharlie, recvGroup)
	ts.checkShare(c, "/receive/group/1?includeme=1&token="+tokenCharlie, recvGroup)
	ts.requestWithMethod(c, "/receive/group/2?token="+tokenCharlie, "GET", http.StatusBadRequest, &common.ErrResponse{}, &reply, nil)
	ts.requestWithMethod(c, "/receive/group/2?includeme=1&token="+tokenCharlie, "GET", http.StatusBadRequest, &common.ErrResponse{}, &reply, nil)
}
