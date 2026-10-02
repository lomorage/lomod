package client

import (
	"fmt"
	"net/http"

	"bitbucket.org/lomoware/lomo-backend/common/group"
)

// AddGroup adds new group
func (ld *Lomod) AddGroup(g *group.Group) error {
	resp, err := ld.requestReply(http.MethodPost, "/group", g)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return checkAndReturnReply(resp)
}

// AddUserInGroup adds new user in one group
func (ld *Lomod) AddUserInGroup(groupID, userID int) error {
	return ld.request(http.MethodPost, fmt.Sprintf("/group/%d/%d", groupID, userID), nil, nil)
}
