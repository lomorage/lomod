package common

import (
	"fmt"
	"sort"

	. "gopkg.in/check.v1"
)

type errInfo struct {
	err    error
	id     int
	status int
}

type byErrID []errInfo

func (a byErrID) Len() int           { return len(a) }
func (a byErrID) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byErrID) Less(i, j int) bool { return a[i].id < a[j].id }

func (ts *commonSuite) TestPrintError(c *C) {
	errs := []errInfo{}
	for err, info := range errMaps {
		errs = append(errs, errInfo{err: err, id: info.id, status: info.status})
	}

	sort.Sort(byErrID(errs))
	for _, err := range errs {
		fmt.Printf("%d\t%d\t%s\n", err.id, err.status, err.err)
	}
}
