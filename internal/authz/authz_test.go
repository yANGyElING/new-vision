package authz

import (
	"testing"
)

func TestAllowMatrix(t *testing.T) {
	cases := []struct {
		roles []string
		obj   string
		act   string
		want  bool
	}{
		// node_admin: everything
		{[]string{"node_admin"}, ObjIdentity, ActManage, true},
		{[]string{"node_admin"}, ObjOrgUnit, ActManage, true},
		{[]string{"node_admin"}, ObjDevice, ActDelete, true},
		{[]string{"node_admin"}, ObjTestSIP, ActRegister, true},
		// tenant_admin: org_unit manage (own tenant), device full, no identity, no test:sip
		{[]string{"tenant_admin"}, ObjOrgUnit, ActManage, true},
		{[]string{"tenant_admin"}, ObjDevice, ActCreate, true},
		{[]string{"tenant_admin"}, ObjDevice, ActDelete, true},
		{[]string{"tenant_admin"}, ObjIdentity, ActManage, false},
		{[]string{"tenant_admin"}, ObjTestSIP, ActRegister, false},
		// operator: device view/enable, access view/events
		{[]string{"operator"}, ObjDevice, ActView, true},
		{[]string{"operator"}, ObjDevice, ActEnable, true},
		{[]string{"operator"}, ObjDevice, ActDelete, false},
		{[]string{"operator"}, ObjAccess, ActEvents, true},
		{[]string{"operator"}, ObjOrgUnit, ActManage, false},
		// viewer: device/access view only
		{[]string{"viewer"}, ObjDevice, ActView, true},
		{[]string{"viewer"}, ObjDevice, ActEnable, false},
		{[]string{"viewer"}, ObjAccess, ActView, true},
		// unknown role / empty roles
		{[]string{"unknown"}, ObjDevice, ActView, false},
		{[]string{}, ObjDevice, ActView, false},
	}
	for _, c := range cases {
		if got := Allow(c.roles, c.obj, c.act); got != c.want {
			t.Errorf("Allow(%v, %q, %q) = %v, want %v", c.roles, c.obj, c.act, got, c.want)
		}
	}
}

func TestAuthorize(t *testing.T) {
	if !Authorize([]string{"node_admin"}, ObjOrgUnit, ActManage) {
		t.Fatal("node_admin should manage org units")
	}
	if Authorize([]string{"viewer"}, ObjOrgUnit, ActManage) {
		t.Fatal("viewer must not manage org units")
	}
}
