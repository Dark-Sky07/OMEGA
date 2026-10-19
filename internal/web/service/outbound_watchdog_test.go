package service

import (
	"encoding/json"
	"testing"
)

func TestExtractWatchdogCandidatesAndPromote(t *testing.T) {
	tpl := `{
  "outbounds": [
    {"tag": "windscribe-wg-1", "protocol": "wireguard"},
    {"tag": "windscribe-wg-2", "protocol": "wireguard"},
    {"tag": "direct", "protocol": "freedom"},
    {"tag": "blocked", "protocol": "blackhole"}
  ],
  "routing": {
    "rules": [
      {"type": "field", "inboundTag": ["inbound-443"], "outboundTag": "windscribe-wg-1"}
    ]
  }
}`

	cands, active := extractWatchdogCandidates(tpl, "")
	if len(cands) != 2 || cands[0] != "windscribe-wg-1" || cands[1] != "windscribe-wg-2" {
		t.Fatalf("unexpected candidates: %v", cands)
	}
	if active != "windscribe-wg-1" {
		t.Fatalf("expected active=windscribe-wg-1, got %q", active)
	}

	updated, changed, err := promoteOutboundInTemplate(tpl, "windscribe-wg-1", "windscribe-wg-2")
	if err != nil || !changed {
		t.Fatalf("promoteOutboundInTemplate failed: changed=%v err=%v", changed, err)
	}

	candsAfter, activeAfter := extractWatchdogCandidates(updated, "windscribe-wg-1,windscribe-wg-2")
	if len(candsAfter) != 2 || activeAfter != "windscribe-wg-2" {
		t.Fatalf("expected activeAfter=windscribe-wg-2, got %q (cands=%v)", activeAfter, candsAfter)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(updated), &parsed); err != nil {
		t.Fatalf("unmarshal updated: %v", err)
	}
	routing := parsed["routing"].(map[string]any)
	rules := routing["rules"].([]any)
	rule0 := rules[0].(map[string]any)
	if rule0["outboundTag"] != "windscribe-wg-2" {
		t.Fatalf("expected routing rule outboundTag updated to windscribe-wg-2, got %v", rule0["outboundTag"])
	}
}
