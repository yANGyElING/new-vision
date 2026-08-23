package authz

// This file intentionally contains only the role loader type. The permission
// matrix and Allow live in model.go; the middleware in middleware.go.
//
// Authorize checks whether the user with the given roles may perform act on
// obj via the constant matrix.
func Authorize(roles []string, obj, act string) bool {
	return Allow(roles, obj, act)
}