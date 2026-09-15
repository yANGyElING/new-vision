package authz

// Permission points (obj, act) used by the node-app HTTP surface.
const (
	ObjDevice   = "device"
	ObjAccess   = "access"
	ObjTestSIP  = "test:sip"
	ObjIdentity = "identity"
	ObjOrgUnit  = "org_unit"
	ObjChannel  = "channel"

	ActCreate     = "create"
	ActView       = "view"
	ActUpdate     = "update"
	ActEnable     = "enable"
	ActDelete     = "delete"
	ActEvents     = "events"
	ActAck        = "ack"
	ActRegister   = "register"
	ActKeepalive  = "keepalive"
	ActUnregister = "unregister"
	ActManage     = "manage"
)

// rolePermissions is the fixed role -> permission point matrix.
// A nil slice means no access to that object.
var rolePermissions = map[string]map[string][]string{
	"node_admin": {
		ObjDevice:   {ActCreate, ActView, ActUpdate, ActEnable, ActDelete},
		ObjAccess:   {ActView, ActEvents, ActAck},
		ObjTestSIP:  {ActRegister, ActKeepalive, ActUnregister},
		ObjIdentity: {ActManage},
		ObjOrgUnit:  {ActManage},
		ObjChannel:  {ActView},
	},
	"tenant_admin": {
		ObjDevice:  {ActCreate, ActView, ActUpdate, ActEnable, ActDelete},
		ObjAccess:  {ActView, ActEvents, ActAck},
		ObjOrgUnit: {ActManage},
		ObjChannel: {ActView},
	},
	"operator": {
		ObjDevice:  {ActView, ActEnable},
		ObjAccess:  {ActView, ActEvents},
		ObjChannel: {ActView},
	},
	"viewer": {
		ObjDevice:  {ActView},
		ObjAccess:  {ActView},
		ObjChannel: {ActView},
	},
}

// Allow reports whether any of the given roles may perform act on obj.
// The matrix is a compile-time constant; no rule engine is involved.
func Allow(roles []string, obj, act string) bool {
	for _, role := range roles {
		if acts, ok := rolePermissions[role]; ok {
			for _, a := range acts[obj] {
				if a == act {
					return true
				}
			}
		}
	}
	return false
}

// AllRoles returns the fixed role names in a stable order.
func AllRoles() []string {
	return []string{"node_admin", "tenant_admin", "operator", "viewer"}
}
