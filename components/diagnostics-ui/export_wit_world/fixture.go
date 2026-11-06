package export_wit_world

import (
	"errors"
	"fmt"

	"wit_component/ohrats_rc_diagnostics_query"
	"wit_component/ohrats_rc_webui_slots"
	"wit_component/ohrats_rc_webui_types"
)

const pageID = "diagnostics"

func activate() error {
	health := ohrats_rc_diagnostics_query.Status()
	result := ohrats_rc_webui_slots.RegisterPage(ohrats_rc_webui_types.Page{
		Id:      pageID,
		Title:   "Diagnostics",
		Path:    "/diagnostics",
		Summary: "Bounded component health and error metadata",
		Content: fmt.Sprintf(
			"Retained: %d\nWarnings: %d\nErrors: %d\nNewest sequence: %d",
			health.Retained, health.Warnings, health.Errors, health.NewestSequence,
		),
	})
	if result.IsErr() {
		return errors.New(result.Err())
	}
	return nil
}

func deactivate() {
	_ = ohrats_rc_webui_slots.RemovePage(pageID)
}
