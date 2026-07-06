package localui

type restorePlan struct {
	OK              bool           `json:"ok"`
	ConfirmRequired bool           `json:"confirm_required"`
	Errors          []string       `json:"errors"`
	Request         restoreRequest `json:"request"`
}

func planRestore(req restoreRequest) restorePlan {
	if req.SnapshotID == "" {
		req.SnapshotID = "latest"
	}
	validateErrs, confirmRequired := validateRestoreRequest(req)
	return restorePlan{
		OK:              len(validateErrs) == 0,
		ConfirmRequired: confirmRequired,
		Errors:          validateErrs,
		Request:         req,
	}
}
