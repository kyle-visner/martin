package martin

import "strings"

type accessLevel int

const (
	accessRead accessLevel = iota + 1
	accessWrite
	accessManage
)

func ensureAccess(st State, ctx Context, required accessLevel) error {
	actor := strings.TrimSpace(ctx.Actor)
	if actor == "" {
		return appErr(ErrPermission, "actor is required")
	}
	roleName := ""
	if user, ok := st.MagpieUsers[actor]; ok {
		roleName = user.Role
		if ctx.Role != "" && ctx.Role != roleName {
			return appErr(ErrPermission, "actor %q cannot assume role %q", actor, ctx.Role)
		}
	} else if len(st.MagpieUsers) == 0 && actor == "owner" {
		roleName = "Owner"
		if ctx.Role != "" && ctx.Role != roleName {
			return appErr(ErrPermission, "actor %q cannot assume role %q", actor, ctx.Role)
		}
	} else {
		return appErr(ErrPermission, "unknown actor %q", actor)
	}

	granted := accessLevel(0)
	switch roleName {
	case "Owner", "Admin":
		granted = accessManage
	case "Sales Rep":
		granted = accessWrite
	case "Operations", "Accountant":
		granted = accessRead
	default:
		return appErr(ErrPermission, "role %q has no Martin access", roleName)
	}
	if granted < required {
		return appErr(ErrPermission, "role %q lacks required Martin access", roleName)
	}
	return nil
}

func ensureReady(st State, ctx Context, required accessLevel) error {
	if st.Workspace == nil {
		return appErr(ErrValidation, "Martin is not initialized; run martin init")
	}
	return ensureAccess(st, ctx, required)
}
