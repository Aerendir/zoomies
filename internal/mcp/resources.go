package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// Resources are what an agent loads to learn the vocabulary before it works,
// without spending a tool call: the catalog's index at one URI, and any entry
// by its code through a template, and the fleet's counts as they are now. Each
// reads the route its tool reads (get_catalog, fleet_status), with the same
// token, so a resource is never a way round a tool's limits.

const (
	catalogURI      = "zoomies://catalog"
	catalogTemplate = "zoomies://catalog/{code}"
	catalogMIME     = "application/json"
	// fleetNowURI is the overview as it is this moment, the same answer
	// fleet_status gives with no window, for an agent that wants the numbers
	// before it decides which tool to call.
	fleetNowURI = "zoomies://fleet/now"
	// rpcResourceNotFound is the code the specification reserves for a URI
	// the server does not serve, so a client can tell it from a bad request.
	rpcResourceNotFound = -32002
)

// resource is one thing the server serves by URI, or a template for a family
// of them. The list is data so the reference and the protocol's two listings
// cannot disagree about what is offered.
type resource struct {
	uri, name, title, description string
	template                      bool
}

var resources = []resource{
	{
		uri:   catalogURI,
		name:  "catalog",
		title: "Catalog of problem codes and checks",
		description: "Every problem code and Kennel Club check this fleet can report, by category. " +
			"Read " + catalogTemplate + " for one entry in full: what it means, what to change and how to see that it worked.",
	},
	{
		uri:   fleetNowURI,
		name:  "fleet-now",
		title: "The fleet as it is now",
		description: "The fleet at a glance as the controller counts it now: jobs queued and running, completed jobs by outcome, queue waits, runners by state and each pool's utilisation. " +
			"The same answer as fleet_status with no window; call recent_events for what moved to get here.",
	},
	{
		uri:         catalogTemplate,
		template:    true,
		name:        "catalog-entry",
		title:       "One catalog entry",
		description: "One problem code or check in full, by the code list_problems, get_job or kennel_findings show, such as pool.no_host or ci.no_timeout.",
	},
}

func (s *Server) listResources() any {
	out := []map[string]any{}
	for _, r := range resources {
		if !r.template {
			out = append(out, map[string]any{"uri": r.uri, "name": r.name, "title": r.title, "mimeType": catalogMIME, "description": r.description})
		}
	}
	return map[string]any{"resources": out}
}

func (s *Server) listResourceTemplates() any {
	out := []map[string]any{}
	for _, r := range resources {
		if r.template {
			out = append(out, map[string]any{"uriTemplate": r.uri, "name": r.name, "title": r.title, "mimeType": catalogMIME, "description": r.description})
		}
	}
	return map[string]any{"resourceTemplates": out}
}

func (s *Server) readResource(ctx context.Context, params json.RawMessage) (any, *RPCError) {
	var p struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.URI == "" {
		return nil, &RPCError{rpcInvalidParams, "resources/read wants a uri"}
	}
	if p.URI == fleetNowURI {
		body, err := s.api.Call(ctx, http.MethodGet, "/stats", nil)
		if err != nil {
			return nil, &RPCError{rpcInternalError, err.Error()}
		}
		return map[string]any{"contents": []map[string]any{{"uri": p.URI, "mimeType": catalogMIME, "text": jsonContent(body)[0].Text}}}, nil
	}
	code, isEntry := strings.CutPrefix(p.URI, catalogURI+"/")
	if p.URI != catalogURI && (!isEntry || code == "") {
		return nil, &RPCError{rpcResourceNotFound, "no resource at " + strconv.Quote(p.URI) + "; this server serves " + catalogURI + ", " + catalogTemplate + " and " + fleetNowURI}
	}
	cat, err := readCatalog(ctx, s.api)
	if err != nil {
		return nil, &RPCError{rpcInternalError, err.Error()}
	}
	var text []byte
	if isEntry {
		entry, ok := cat.entry(code)
		if !ok {
			return nil, &RPCError{rpcResourceNotFound, "there is no " + code + " in the catalog; " + catalogURI + " lists every code"}
		}
		text = entry
	} else if text, err = cat.index(); err != nil {
		return nil, &RPCError{rpcInternalError, err.Error()}
	}
	return map[string]any{"contents": []map[string]any{{"uri": p.URI, "mimeType": catalogMIME, "text": string(text)}}}, nil
}
