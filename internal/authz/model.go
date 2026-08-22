package authz

// CasbinModel is the PERM model for node-app authorization.
// RBAC with domains: sub is the user, dom is the tenant id, obj is a
// permission-point resource (device / access / test:sip / identity),
// act is the action (create / view / update / enable / delete / ...).
const CasbinModel = `
[request_definition]
r = sub, dom, obj, act

[policy_definition]
p = sub, dom, obj, act

[role_definition]
g = _, _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub, r.dom) && r.dom == p.dom && keyMatch(r.obj, p.obj) && r.act == p.act
`

// Permission points (obj, act) used by the node-app HTTP surface.
const (
	ObjDevice = "device"
	ObjAccess = "access"
	ObjTestSIP = "test:sip"
	ObjIdentity = "identity"

	ActCreate   = "create"
	ActView     = "view"
	ActUpdate   = "update"
	ActEnable   = "enable"
	ActDelete   = "delete"
	ActEvents   = "events"
	ActAck      = "ack"
	ActRegister = "register"
	ActKeepalive = "keepalive"
	ActUnregister = "unregister"
	ActManage   = "manage"
)

// rolePermission is the fixed role -> permission point matrix.
// A nil slice means no access to that object.
var rolePermissions = map[string]map[string][]string{
	"node_admin": {
		ObjDevice:   {ActCreate, ActView, ActUpdate, ActEnable, ActDelete},
		ObjAccess:   {ActView, ActEvents, ActAck},
		ObjTestSIP:  {ActRegister, ActKeepalive, ActUnregister},
		ObjIdentity: {ActManage},
	},
	"tenant_admin": {
		ObjDevice: {ActCreate, ActView, ActUpdate, ActEnable, ActDelete},
		ObjAccess: {ActView, ActEvents, ActAck},
	},
	"operator": {
		ObjDevice: {ActView, ActEnable},
		ObjAccess: {ActView, ActEvents},
	},
	"viewer": {
		ObjDevice: {ActView},
		ObjAccess: {ActView},
	},
}

// AllRoles returns the fixed role names in a stable order.
func AllRoles() []string {
	return []string{"node_admin", "tenant_admin", "operator", "viewer"}
}
