package tools

import (
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestParseInputSchemaHandlesOpenAPITypes(t *testing.T) {
	raw := json.RawMessage(`{
		"type":"object",
		"required":["block_id","parent"],
		"properties":{
			"block_id":{"type":"string"},
			"parent":{"type":["object","null"],"properties":{"page_id":{"type":"string"}},"required":["page_id"]},
			"kind":{"const":"workspace"}
		}
	}`)

	params := parseInputSchema(raw)
	if params["block_id"] == nil || !params["block_id"].Required || params["block_id"].Type != schema.String {
		t.Fatalf("block_id parsed incorrectly: %#v", params["block_id"])
	}
	if params["parent"] == nil || params["parent"].Type != schema.Object || params["parent"].SubParams["page_id"] == nil || !params["parent"].SubParams["page_id"].Required {
		t.Fatalf("parent parsed incorrectly: %#v", params["parent"])
	}
	if params["kind"] == nil || params["kind"].Type != schema.String || len(params["kind"].Enum) != 1 || params["kind"].Enum[0] != "workspace" {
		t.Fatalf("const parsed incorrectly: %#v", params["kind"])
	}
}
