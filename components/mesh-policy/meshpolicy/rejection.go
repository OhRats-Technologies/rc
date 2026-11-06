package meshpolicy

import "wit_component/ohrats_rc_mesh_types"

// rejectionError carries a WIT rejection through internal policy helpers.
// Exported entry points translate it back into the WIT result type.
type rejectionError struct {
	value ohrats_rc_mesh_types.Rejection
}

func (e rejectionError) Error() string {
	return "mesh policy rejection"
}

func reject(value ohrats_rc_mesh_types.Rejection) error {
	return rejectionError{value}
}

func rejectionOf(err error) ohrats_rc_mesh_types.Rejection {
	if rejection, ok := err.(rejectionError); ok {
		return rejection.value
	}
	return ohrats_rc_mesh_types.MakeRejectionInvalidEnvelope()
}
