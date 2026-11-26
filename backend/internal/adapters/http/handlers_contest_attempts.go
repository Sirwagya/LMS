package httpadapter

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"vedam/lms/internal/domain/contest"
)

func (a *api) contestAttempt(w http.ResponseWriter, r *http.Request) error {
	// This identifies the supported transport, without claiming device attestation.
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Authorization")), "bearer ") ||
		r.Header.Get("X-LMS-Contest-Client") != "macos" ||
		!nativeRequest(r) {
		return problem(403, "Open this contest in the LMS macOS app.")
	}
	id, err := contestID(r)
	if err != nil {
		return err
	}
	f := readBody(w, r, 1<<20)
	if f.err != nil {
		return f.err
	}
	action := f.optional("action")
	device, err := uuid.Parse(f.optional("deviceId"))
	if err != nil || device.Version() != 4 {
		return invalid("The app must provide a valid device session.")
	}
	cmd := contest.AttemptCommand{Action: action, DeviceID: device.String()}
	switch action {
	case "state", "start":
	case "takeover", "save", "submit":
		if json.Unmarshal(f.values["lease"], &cmd.Lease) != nil || cmd.Lease < 1 {
			return invalid("Provide the current device lease.")
		}
		if action != "takeover" {
			var revision *int64
			if json.Unmarshal(f.values["revision"], &revision) != nil || revision == nil || *revision < 0 ||
				json.Unmarshal(f.values["answers"], &cmd.Answers) != nil || cmd.Answers == nil {
				return invalid("Provide answers and their current revision.")
			}
			cmd.Revision = *revision
		}
	default:
		return invalid("Unknown contest attempt action.")
	}

	view, err := a.ContestAttempts.Execute(r.Context(), id, actor(r).UserID, cmd)
	if err != nil {
		return missing(err, "Contest not found.")
	}
	writeJSON(w, 200, view)
	return nil
}
