package handler

import (
	"bytes"
	"encoding/json"
	"net/http"

	"bitbucket.org/lomoware/lomo-backend/common"
	"bitbucket.org/lomoware/lomo-backend/common/group"
	"bitbucket.org/lomoware/lomo-backend/common/user"
	. "gopkg.in/check.v1"
)

func (ts *mainSuite) TestGroup(c *C) {
	ts.requestWithMethod(c, "/group", "GET", http.StatusUnauthorized, &common.ErrResponse{}, &invalidToken, nil)
	gs := group.Groups{Groups: []group.Group{}}
	ts.requestGet(c, "/group?token="+ts.token, &group.Groups{}, &gs)

	// create single group
	ts.requestWithMethod(c, "/group/group1?token="+ts.token, "POST", http.StatusOK, nil, nil, nil)

	// create 2 groups by json
	g := group.Group{OwnerID: 1, Name: "group2", Members: []*user.User{{ID: 2}}}
	content, err := json.Marshal(g)
	c.Assert(err, IsNil)

	// if owner is alice, bob can not create the group
	reply := common.ErrResponse{ID: "7", Text: common.ErrDifferentUsername.Error()}
	ts.requestWithMethodBody(c, "/group?token="+ts.tokenBob, "POST", http.StatusBadRequest,
		bytes.NewBuffer(content), &common.ErrResponse{}, &reply, nil)

	ts.requestWithMethodBody(c, "/group?token="+ts.token, "POST", http.StatusOK, bytes.NewBuffer(content),
		nil, nil, nil)

	// post again should fail
	reply.ID = "18"
	reply.Text = common.ErrGroupExist.Error()
	ts.requestWithMethodBody(c, "/group?token="+ts.token, "POST", http.StatusBadRequest, bytes.NewBuffer(content),
		&common.ErrResponse{}, &reply, nil)

	gs.Groups = []group.Group{
		{ID: 1, OwnerID: 1, Name: "group1", Members: []*user.User{{ID: 1, Name: "alice"}}},
		{ID: 2, OwnerID: 1, Name: "group2", Members: []*user.User{{ID: 1, Name: "alice"}, {ID: 2, Name: "bob"}}},
	}
	ts.requestGet(c, "/group?token="+ts.token, &group.Groups{}, &gs)

	// nonexist group should return failure
	reply.ID = "1"
	reply.Text = common.ErrBadRequest.Error()
	ts.requestWithMethod(c, "/group/notexist?token="+ts.token, "GET", http.StatusBadRequest,
		&common.ErrResponse{}, &reply, nil)

	ts.requestGet(c, "/group/1?token="+ts.token, &[]*user.User{}, &gs.Groups[0].Members)
	ts.requestGet(c, "/group/2?token="+ts.token, &[]*user.User{}, &gs.Groups[1].Members)

	// add one user
	ts.requestWithMethod(c, "/group/1/2?token="+ts.token, "POST", http.StatusOK, nil, nil, nil)

	us := append(gs.Groups[0].Members, &user.User{ID: 2, Name: "bob"})
	ts.requestGet(c, "/group/1?token="+ts.token, &[]*user.User{}, &us)

	// add the user again should get duplicate
	reply.ID = "2"
	reply.Text = common.ErrDuplicate.Error()
	ts.requestWithMethod(c, "/group/1/2?token="+ts.token, "POST", http.StatusBadRequest,
		&common.ErrResponse{}, &reply, nil)

	// delete the user from group
	ts.requestWithMethod(c, "/group/1/2?token="+ts.token, "DELETE", http.StatusOK, nil, nil, nil)
	ts.requestGet(c, "/group/1?token="+ts.token, &[]*user.User{}, &gs.Groups[0].Members)

	// delete the user from group again
	ts.requestWithMethod(c, "/group/1/2?token="+ts.token, "DELETE", http.StatusOK, nil, nil, nil)
	ts.requestGet(c, "/group/1?token="+ts.token, &[]*user.User{}, &gs.Groups[0].Members)
}
