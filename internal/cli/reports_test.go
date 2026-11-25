package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestMilestoneSchemaNeedsNoConfigurationOrCredentials(t *testing.T) {
	var out bytes.Buffer
	app := App{Out: &out}
	if code := app.Run(context.Background(), []string{"reports", "monthly", "milestone", "schema"}); code != 0 {
		t.Fatalf("exit=%d output=%s", code, out.String())
	}
	var response struct {
		Data struct {
			RequestBody struct {
				Required   []string `json:"required"`
				Properties map[string]struct {
					Type string   `json:"type"`
					Enum []string `json:"enum"`
				} `json:"properties"`
			} `json:"requestBody"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Data.RequestBody.Required) != 2 || len(response.Data.RequestBody.Properties["status"].Enum) != 6 || response.Data.RequestBody.Properties["notes"].Type != "string" {
		t.Fatalf("incomplete contract: %s", out.String())
	}
}
