package permissions

type SharePermissions int16

const (
	// 1 << 0 to 1 (Bin: 001)
	PermissionRead SharePermissions = 1 << iota

	// 1 << 1 = 2 (Bin: 010)
	PermissionWrite

	// 1 << 2 = 4 (Bin: 100)
	PermissionAddMembers
)

const (
	// Write-Only: 1
	RoleViewer = PermissionRead

	// Read-Write: 1 + 2 = 3
	RoleEditor = PermissionRead | PermissionWrite

	// Read-Write-Add: 1 + 2 + 4 = 7
	RoleAdmin = PermissionRead | PermissionWrite | PermissionAddMembers
)

func NewPermissions(write bool, addMembers bool) SharePermissions {
	p := PermissionRead

	if write {
		p |= PermissionWrite
	}
	if addMembers {
		p |= PermissionAddMembers
	}

	return p
}

type SharePermissionsJSON struct {
	Read       bool `json:"read"`
	Write      bool `json:"write"`
	AddMembers bool `json:"addMembers"`
}

func (p SharePermissions) ToJSON() SharePermissionsJSON {
	return SharePermissionsJSON{
		Read:       p&PermissionRead != 0,
		Write:      p&PermissionWrite != 0,
		AddMembers: p&PermissionAddMembers != 0,
	}
}
