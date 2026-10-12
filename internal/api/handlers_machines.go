package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/eyupio/zoomies/internal/controller"
	"github.com/eyupio/zoomies/internal/store"
)

// machineResponse is the shape GET /machines returns, rendered by the
// controller so the event stream's machine.updated frames are the same JSON.
type machineResponse = controller.MachineView

// machineTimelineResponse is one phase a machine reached.
type machineTimelineResponse = controller.MachineTimelineEntry

// There is deliberately no POST /machines.
//
// A machine exists because demand asked for one: one creation path means one
// accounting path, and a machine made by hand would be a second source of
// supply that the reconciler -- which decides how many should exist from the
// demand it can see -- would then decide to delete. An operator who wants more
// machines raises the provider's ceiling, and an operator who wants one for
// something else creates it in their hypervisor, where Zoomies will leave it
// alone because it carries none of its marks.

// handleListMachines answers GET /api/v1/machines.
func (s *Server) handleListMachines(w http.ResponseWriter, r *http.Request) {
	filter := store.MachineFilter{
		ProviderIDs:    queryList(r, "provider"),
		PoolIDs:        queryList(r, "pool"),
		HostIDs:        queryList(r, "host"),
		Search:         strings.TrimSpace(r.URL.Query().Get("q")),
		IncludeDeleted: queryBool(r, "include_deleted", false),
	}
	for _, raw := range queryList(r, "state") {
		state := store.MachineState(raw)
		if !state.Valid() {
			badRequestField(w, "state", fmt.Sprintf("%q is not a machine state; use planned, creating, starting, "+
				"bootstrapping, enrolling, ready, draining, deleting, deleted, failed or quarantined", raw))
			return
		}
		filter.States = append(filter.States, state)
	}

	page := parsePage(r)
	machines, total, err := s.ctrl.Store().ListMachines(r.Context(), filter, page)
	if err != nil {
		s.internal(w, r, "listing machines", err)
		return
	}
	view, err := s.ctrl.MachineRenderer(r.Context())
	if err != nil {
		s.internal(w, r, "reading what the machines belong to", err)
		return
	}
	out := make([]machineResponse, 0, len(machines))
	for _, m := range machines {
		out = append(out, view.View(m))
	}
	writeJSON(w, http.StatusOK, newPage(out, total, page))
}

// handleGetMachine answers GET /api/v1/machines/{id}.
func (s *Server) handleGetMachine(w http.ResponseWriter, r *http.Request) {
	m, view, ok := s.machineFor(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, view.View(m))
}

// machineFor reads one machine and the renderer its view needs, answering the
// client itself when either read fails.
func (s *Server) machineFor(w http.ResponseWriter, r *http.Request) (*store.Machine, *controller.MachineRenderer, bool) {
	m, err := s.ctrl.Store().GetMachine(r.Context(), chiURLParam(r, "id"))
	if err != nil {
		s.fail(w, r, "reading the machine", err)
		return nil, nil, false
	}
	view, err := s.ctrl.MachineRenderer(r.Context())
	if err != nil {
		s.internal(w, r, "reading what the machine belongs to", err)
		return nil, nil, false
	}
	return m, view, true
}

// cordonHostOf stops the scheduler placing onto the host of a machine that is
// on its way out. A machine with no host has nothing to cordon.
func (s *Server) cordonHostOf(ctx context.Context, m *store.Machine) error {
	if m.HostID == "" {
		return nil
	}
	if err := s.ctrl.Store().SetHostCordoned(ctx, m.HostID, true); err != nil {
		return fmt.Errorf("cordoning the machine's host: %w", err)
	}
	if h, err := s.ctrl.Store().GetHost(ctx, m.HostID); err == nil {
		s.ctrl.PublishHost(h)
	}
	return nil
}

// cordonMachineHost is cordonHostOf for a handler: it answers the client itself
// and reports whether the request may go on.
func (s *Server) cordonMachineHost(w http.ResponseWriter, r *http.Request, m *store.Machine) bool {
	if err := s.cordonHostOf(r.Context(), m); err != nil {
		s.fail(w, r, "cordoning the machine's host", err)
		return false
	}
	return true
}

// handleDrainMachine answers POST /api/v1/machines/{id}/drain.
//
// Draining is reversible right up until the delete starts: demand coming back
// takes a draining machine to ready again rather than paying for a new one.
// The host is cordoned in the same breath, because a machine on its way out
// that kept accepting runners would never empty.
func (s *Server) handleDrainMachine(w http.ResponseWriter, r *http.Request) {
	m, _, ok := s.machineFor(w, r)
	if !ok {
		return
	}
	// The transition is checked before the cordon: a refused drain (only a ready
	// machine can drain) must not leave the host of a failed or deleting machine
	// cordoned by a request that answered 409 and wrote no audit row.
	if !store.CanTransitionMachine(m.State, store.MachineDraining) {
		s.fail(w, r, "draining the machine", fmt.Errorf("%w: machine %s cannot go %s -> %s",
			store.ErrInvalidTransition, m.ID, m.State, store.MachineDraining))
		return
	}
	if !s.cordonMachineHost(w, r, m) {
		return
	}
	out, err := s.ctrl.Store().TransitionMachine(r.Context(), m.ID, store.MachineDraining,
		"an operator asked for this machine to be drained")
	if err != nil {
		s.fail(w, r, "draining the machine", err)
		return
	}
	s.auth.Auditor().Act(r.Context(), Identity(r.Context()), "machine.drain", "machine", m.ID, map[string]any{
		"name": m.Name, "was": m.State, "host_id": m.HostID,
	})
	s.publishMachine(r, out)
	writeJSON(w, http.StatusOK, s.machineView(r, out))
}

// handleDeleteMachine answers DELETE /api/v1/machines/{id}.
//
// It records the intent and lets the machine loop carry it out, because a
// delete is not finished when the provider returns: it is finished when an
// inspect can no longer find the resource. So this answers with the machine in
// deleting rather than with 204, and the page watches it reach deleted.
func (s *Server) handleDeleteMachine(w http.ResponseWriter, r *http.Request) {
	m, _, ok := s.machineFor(w, r)
	if !ok {
		return
	}
	out, err := s.removeMachine(r.Context(), m, queryBool(r, "force", false), false)
	var refusal *machineRefusal
	switch {
	case errors.As(err, &refusal):
		reason := refusal.reason
		if refusal.forceable {
			reason += " Drain it and call this again when they have finished, " +
				"or repeat this with ?force=true to delete it now and lose that work."
		}
		conflict(w, reason)
		return
	case err != nil:
		s.fail(w, r, "deleting the machine", err)
		return
	}
	s.publishMachine(r, out)
	writeJSON(w, http.StatusOK, s.machineView(r, out))
}

// machineRefusal is a delete the rules turn down, as opposed to one that
// failed. It changes nothing, which is what lets a bulk request report it
// against one machine and carry on with the rest.
type machineRefusal struct {
	reason string
	// forceable says that insisting would have changed the answer. The words for
	// insisting differ by door (a query string on one machine, a field in a bulk
	// body), so each caller adds its own rather than this carrying syntax that is
	// wrong for the other.
	forceable bool
}

func (e *machineRefusal) Error() string { return e.reason }

// removeMachine puts a machine on its way to being deleted, or says why not.
//
// It is the one copy of the rules, shared by DELETE /machines/{id} and the bulk
// route, so the two cannot come to disagree about what may be deleted: a rule
// added to one door and forgotten at the other would let a request destroy a VM
// that the other refuses to touch.
//
// A refusal comes back as a *machineRefusal and has changed nothing, the host
// included. Any other error is a failure to look or to write. Announcing the
// machine and nudging the machine loop are the caller's, so that a request that
// deletes forty machines asks the loop to look once rather than forty times.
func (s *Server) removeMachine(ctx context.Context, m *store.Machine, force, bulk bool) (*store.Machine, error) {
	switch m.State {
	case store.MachineQuarantined:
		// A quarantined machine is never deleted from here, forced or not. Its
		// ownership could not be proved, so the resource behind it may belong to
		// another fleet, and a delete would destroy somebody else's machine. The
		// escape hatch is release, which touches nothing.
		return nil, &machineRefusal{reason: fmt.Sprintf("machine %s is quarantined: nothing has been able to prove the resource behind it is ours, "+
			"so deleting it could destroy a machine belonging to somebody else. Look at %s in the provider yourself. "+
			"If it is ours, remove it there; if it is not, POST /api/v1/machines/%s/release to forget this row, which touches no resource.",
			m.Name, resourceLabel(m), m.ID)}
	case store.MachineDeleted:
		// Said in words because the alternative is the state machine's own
		// complaint, "cannot go deleted -> deleting", which is true and tells an
		// operator nothing about what to do. The same goes for planned below.
		return nil, &machineRefusal{reason: fmt.Sprintf("machine %s is already deleted: the provider confirmed its resource gone, "+
			"so there is nothing left to remove.", m.Name)}
	case store.MachinePlanned:
		return nil, &machineRefusal{reason: fmt.Sprintf("machine %s is only planned: nothing has been created at the provider yet, "+
			"so there is nothing to delete. If its create is never issued the reservation lapses and it becomes failed, "+
			"which can be deleted.", m.Name)}
	}
	if m.HostID != "" && !force {
		runners, err := s.ctrl.Store().ListRunnersForHost(ctx, m.HostID)
		if err != nil {
			return nil, fmt.Errorf("listing the machine's runners: %w", err)
		}
		alive := 0
		for _, run := range runners {
			if !run.State.Terminal() {
				alive++
			}
		}
		if alive > 0 {
			return nil, &machineRefusal{forceable: true, reason: fmt.Sprintf("machine %s is still running %s, "+
				"and deleting it destroys the VM they are running on.", m.Name, runnerCount(alive))}
		}
	}

	// Cordoned on the way to deleting, as every other road there does
	// (beginDrain, quarantine, a vanished VM): the scheduler's snapshot carries
	// hosts and never machines, so without it the host stays placeable for the
	// whole delete and a runner placed in that window dies with the VM. The
	// forced path cordons too -- the host is going either way. It comes after
	// the refusals above so that a delete that says no changes nothing.
	if store.CanTransitionMachine(m.State, store.MachineDeleting) {
		if err := s.cordonHostOf(ctx, m); err != nil {
			return nil, err
		}
	}
	out, err := s.ctrl.Store().TransitionMachine(ctx, m.ID, store.MachineDeleting,
		"an operator asked for this machine to be deleted")
	if err != nil {
		return nil, err
	}
	detail := map[string]any{
		"name": m.Name, "was": m.State, "forced": force,
		"resource_zone": m.ResourceZone, "resource_id": m.ResourceID,
	}
	if bulk {
		detail["bulk"] = true
	}
	s.auth.Auditor().Act(ctx, Identity(ctx), "machine.delete", "machine", m.ID, detail)
	return out, nil
}

// maxBulkMachines is the most machines one bulk request may name. It is the
// runners' ceiling, so a script written for one works for the other.
const maxBulkMachines = 500

type bulkMachineRequest struct {
	Action string   `json:"action"`
	IDs    []string `json:"ids"`
	// Force deletes a machine that still has runners on it and loses what they
	// were running, as ?force=true does for one machine. It never reaches a
	// quarantined machine: nothing does.
	Force bool `json:"force"`
}

type bulkMachineResult struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

type bulkMachineResponse struct {
	Results []bulkMachineResult `json:"results"`
}

// handleBulkMachines answers POST /api/v1/machines/bulk.
//
// It answers 200 with a result for each machine, in the order asked, rather than
// one status for the lot. The people who need this are clearing a pile of failed
// machines, and a pile is never uniform: one of them is quarantined, one picked
// up a runner a moment ago, one was deleted by somebody else while the page was
// open. "Something failed" is not an answer anyone can act on.
//
// Each machine goes through removeMachine, the rules a single delete follows.
func (s *Server) handleBulkMachines(w http.ResponseWriter, r *http.Request) {
	var req bulkMachineRequest
	if !decode(w, r, &req) {
		return
	}

	var fields []fieldError
	switch req.Action {
	case "delete":
	case "":
		fields = append(fields, fieldError{"action", "say what to do with these machines: delete"})
	default:
		fields = append(fields, fieldError{"action", fmt.Sprintf("%q is not an action; use delete", req.Action)})
	}
	switch {
	case len(req.IDs) == 0:
		fields = append(fields, fieldError{"ids", "no machines were selected"})
	case len(req.IDs) > maxBulkMachines:
		fields = append(fields, fieldError{"ids", fmt.Sprintf("%d machines is more than the %d this endpoint accepts at once; do it in batches",
			len(req.IDs), maxBulkMachines)})
	}
	if len(fields) > 0 {
		unprocessable(w, "this bulk action could not be carried out", fields)
		return
	}

	ctx := r.Context()
	results := make([]bulkMachineResult, 0, len(req.IDs))
	deleting := 0
	for _, id := range req.IDs {
		if ctx.Err() != nil {
			// The caller has gone, and every read from here on would fail with
			// the same cancellation. What was done stays done and audited.
			break
		}
		m, err := s.ctrl.Store().GetMachine(ctx, id)
		if err == nil {
			m, err = s.removeMachine(ctx, m, req.Force, true)
		}
		if err != nil {
			results = append(results, bulkMachineResult{ID: id, Error: s.machineRowError(r, id, err)})
			continue
		}
		s.ctrl.PublishMachine(ctx, m)
		deleting++
		results = append(results, bulkMachineResult{ID: id, OK: true})
	}
	if deleting > 0 {
		s.ctrl.NudgeMachines()
	}
	writeJSON(w, http.StatusOK, bulkMachineResponse{Results: results})
}

// machineRowError is what a bulk result says about a machine that was not
// deleted. A refusal, and a machine that is not there, are the operator's to act
// on and are said in full. Anything else is logged and answered with the request
// ID, as every other handler does: a row is read by whoever has the page open,
// and the cause is for whoever runs the instance.
func (s *Server) machineRowError(r *http.Request, id string, err error) string {
	var refusal *machineRefusal
	switch {
	case errors.As(err, &refusal):
		if refusal.forceable {
			return refusal.reason + " Drain it first, or force the delete to lose that work."
		}
		return refusal.reason
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrConflict), errors.Is(err, store.ErrInvalidTransition):
		return err.Error()
	}
	requestID := ""
	if info := infoFrom(r.Context()); info != nil {
		requestID = info.id
	}
	s.logger(r).Error("request failed", "doing", "deleting a machine in a bulk request", "machine", id,
		"error", err, "request_id", requestID)
	msg := "something went wrong while deleting this machine. " + supportHint()
	if requestID != "" {
		msg += " Request ID: " + requestID + "."
	}
	return msg
}

// releaseMachineRequest asks for the machine's name to be typed, which is the
// one guard on an act that cannot be undone.
type releaseMachineRequest struct {
	Name string `json:"name"`
}

// handleReleaseMachine answers POST /api/v1/machines/{id}/release: forget this
// row and touch nothing.
//
// It exists for the machine nobody can safely delete -- a quarantined one whose
// resource turned out to belong to somebody else, or a row for a resource that
// was removed in the hypervisor by hand. What it costs is exactly what it says:
// if the resource does still exist, it goes on running and stops being anything
// Zoomies knows about, so the audit row keeps the provider's own identifiers
// for it after the database has stopped holding them.
func (s *Server) handleReleaseMachine(w http.ResponseWriter, r *http.Request) {
	m, _, ok := s.machineFor(w, r)
	if !ok {
		return
	}
	var req releaseMachineRequest
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) != m.Name {
		unprocessable(w, fmt.Sprintf("releasing a machine forgets it without deleting anything, and it cannot be undone: "+
			"send the machine's name, %s, to confirm that is what you mean", m.Name),
			[]fieldError{{"name", fmt.Sprintf("type %s to confirm", m.Name)}})
		return
	}
	if err := s.ctrl.Store().ForgetMachine(r.Context(), m.ID); err != nil {
		s.fail(w, r, "releasing the machine", err)
		return
	}
	s.auth.Auditor().Act(r.Context(), Identity(r.Context()), "machine.release", "machine", m.ID, map[string]any{
		"name": m.Name, "state": m.State, "provider_id": m.ProviderID,
		"resource_zone": m.ResourceZone, "resource_id": m.ResourceID,
		"address": m.Address, "host_id": m.HostID,
	})
	s.ctrl.PublishMachineDeleted(m.ID)
	noContent(w)
}

// publishMachine announces a machine change and asks the machine loop to look
// again, which is what turns "deleting" into a delete rather than a state a row
// sits in until the next interval.
func (s *Server) publishMachine(r *http.Request, m *store.Machine) {
	s.ctrl.PublishMachine(r.Context(), m)
	s.ctrl.NudgeMachines()
}

// machineView renders one machine, falling back to what can be rendered
// without the lookups when they fail: a write that succeeded must not answer as
// though it had not.
func (s *Server) machineView(r *http.Request, m *store.Machine) machineResponse {
	view, err := s.ctrl.MachineRenderer(r.Context())
	if err != nil {
		s.logger(r).Warn("could not render a machine's provider, pool and host names", "machine", m.ID, "error", err)
		return controller.MachineView{ID: m.ID, ProviderID: m.ProviderID, Name: m.Name, State: m.State}
	}
	return view.View(m)
}

// resourceLabel names the resource behind a machine the way its provider does,
// so an operator can paste it into their own console.
func resourceLabel(m *store.Machine) string {
	switch {
	case m.ResourceZone != "" && m.ResourceID != "":
		return m.ResourceZone + "/" + m.ResourceID
	case m.ResourceID != "":
		return m.ResourceID
	}
	return m.Name
}
