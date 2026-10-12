package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func readResourceNamed(t *testing.T, api API, uri string) (map[string]any, *RPCError) {
	t.Helper()
	params, err := json.Marshal(map[string]string{"uri": uri})
	if err != nil {
		t.Fatal(err)
	}
	res, rpcErr := New(api, Options{}).handle(context.Background(), &rpcRequest{Method: "resources/read", Params: params})
	if rpcErr != nil {
		return nil, rpcErr
	}
	return res.(map[string]any), nil
}

// A resource is a way to load the fleet's numbers without a tool call, so it
// reads the route the tool reads, as the caller, and offers nothing more.
func TestFleetNowIsListedAndReadsStats(t *testing.T) {
	api := &kennelAPI{body: "{\n  \"queued\": 3\n}"}
	s := New(api, Options{})

	res, rpcErr := s.handle(context.Background(), &rpcRequest{Method: "resources/list"})
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	var listed []string
	for _, r := range res.(map[string]any)["resources"].([]map[string]any) {
		listed = append(listed, r["uri"].(string))
		if r["uri"] == "zoomies://fleet/now" && (r["name"] != "fleet-now" || r["mimeType"] != "application/json") {
			t.Errorf("fleet-now is listed as %+v", r)
		}
	}
	if len(listed) != 2 || listed[0] != "zoomies://catalog" || listed[1] != "zoomies://fleet/now" {
		t.Errorf("resources/list = %v, want the catalog and the fleet as it is now", listed)
	}

	got, rpcErr := readResourceNamed(t, api, "zoomies://fleet/now")
	if rpcErr != nil {
		t.Fatal(rpcErr)
	}
	if api.got != "GET /stats" || len(api.query) != 0 {
		t.Errorf("asked %q %v, want GET /stats with no window so it is the fleet as it is now", api.got, api.query)
	}
	contents := got["contents"].([]map[string]any)
	if len(contents) != 1 || contents[0]["uri"] != "zoomies://fleet/now" || contents[0]["mimeType"] != "application/json" || contents[0]["text"] != `{"queued":3}` {
		t.Errorf("contents = %+v", contents)
	}
}

// The caller's own token decides what /stats answers, so a refusal is the
// controller's sentence and not an empty fleet.
func TestFleetNowSaysWhenTheCallerMayNotReadStats(t *testing.T) {
	api := &kennelAPI{err: errors.New("your token has no role that reads the fleet")}
	if _, rpcErr := readResourceNamed(t, api, "zoomies://fleet/now"); rpcErr == nil || rpcErr.Code != rpcInternalError || !strings.Contains(rpcErr.Message, "no role that reads the fleet") {
		t.Errorf("got %+v", rpcErr)
	}
}

// The catalog still answers beside it, and a URI neither serves names both.
func TestReadingAnUnknownResourceNamesWhatIsServed(t *testing.T) {
	_, rpcErr := readResourceNamed(t, &kennelAPI{}, "zoomies://nothing")
	if rpcErr == nil || rpcErr.Code != rpcResourceNotFound || !strings.Contains(rpcErr.Message, fleetNowURI) || !strings.Contains(rpcErr.Message, catalogURI) {
		t.Errorf("got %+v", rpcErr)
	}
}
