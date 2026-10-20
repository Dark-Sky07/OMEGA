package sub

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/Dark-Sky07/phoenix/internal/database/model"
	"github.com/Dark-Sky07/phoenix/internal/util/random"
)

func needsSmartHostResolution(s string) bool {
	return strings.Contains(s, ",") ||
		strings.Contains(s, "\n") ||
		strings.Contains(s, "*.") ||
		strings.Contains(s, "{")
}

func splitSmartHostPool(raw string) []string {
	if !strings.Contains(raw, ",") && !strings.Contains(raw, "\n") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return nil
		}
		return []string{trimmed}
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		t := strings.TrimSpace(f)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func applySmartHostTemplates(val string, inbound *model.Inbound, email string, salt string) string {
	if val == "" {
		return ""
	}
	if strings.HasPrefix(val, "*.") {
		if salt == "" {
			salt = random.NumLower(6)
		}
		val = salt + val[1:]
	}
	if !strings.Contains(val, "{") {
		return val
	}
	if salt == "" {
		salt = random.NumLower(6)
	}
	protocol := ""
	port := ""
	remark := ""
	if inbound != nil {
		protocol = string(inbound.Protocol)
		port = fmt.Sprintf("%d", inbound.Port)
		remark = inbound.Remark
	}
	replacer := strings.NewReplacer(
		"{USERNAME}", email,
		"{EMAIL}", email,
		"{PROTOCOL}", protocol,
		"{PORT}", port,
		"{REMARK}", remark,
		"{RAND}", salt,
	)
	return replacer.Replace(val)
}

func resolveSmartExternalProxyList(rawList []any, inbound *model.Inbound, email string) []any {
	if len(rawList) == 0 {
		return rawList
	}
	anyDynamic := false
	for _, item := range rawList {
		ep, ok := item.(map[string]any)
		if !ok {
			continue
		}
		dest, _ := ep["dest"].(string)
		sni, _ := ep["sni"].(string)
		remark, _ := ep["remark"].(string)
		if needsSmartHostResolution(dest) || needsSmartHostResolution(sni) || strings.Contains(remark, "{") {
			anyDynamic = true
			break
		}
	}
	if !anyDynamic {
		return rawList
	}

	out := make([]any, 0, len(rawList))
	for _, item := range rawList {
		ep, ok := item.(map[string]any)
		if !ok {
			out = append(out, item)
			continue
		}
		cloned := make(map[string]any, len(ep))
		for k, v := range ep {
			cloned[k] = v
		}
		destRaw, _ := cloned["dest"].(string)
		sniRaw, _ := cloned["sni"].(string)
		remarkRaw, _ := cloned["remark"].(string)

		destPool := splitSmartHostPool(destRaw)
		sniPool := splitSmartHostPool(sniRaw)

		maxPool := len(destPool)
		if len(sniPool) > maxPool {
			maxPool = len(sniPool)
		}
		pickIdx := 0
		if maxPool > 1 {
			pickIdx = rand.IntN(maxPool)
		}
		salt := random.NumLower(6)

		if len(destPool) > 0 {
			pickedDest := destPool[pickIdx%len(destPool)]
			cloned["dest"] = applySmartHostTemplates(pickedDest, inbound, email, salt)
		}
		if len(sniPool) > 0 {
			pickedSNI := sniPool[pickIdx%len(sniPool)]
			cloned["sni"] = applySmartHostTemplates(pickedSNI, inbound, email, salt)
		}
		if strings.Contains(remarkRaw, "{") {
			cloned["remark"] = applySmartHostTemplates(remarkRaw, inbound, email, salt)
		}
		out = append(out, cloned)
	}
	return out
}

func resolveInboundSmartStreamSettings(inbound *model.Inbound, email string) *model.Inbound {
	if inbound == nil || inbound.StreamSettings == "" {
		return inbound
	}
	if !strings.Contains(inbound.StreamSettings, "externalProxy") {
		return inbound
	}
	if !needsSmartHostResolution(inbound.StreamSettings) {
		return inbound
	}
	var stream map[string]any
	if err := json.Unmarshal([]byte(inbound.StreamSettings), &stream); err != nil {
		return inbound
	}
	rawList, ok := stream["externalProxy"].([]any)
	if !ok || len(rawList) == 0 {
		return inbound
	}
	resolved := resolveSmartExternalProxyList(rawList, inbound, email)
	stream["externalProxy"] = resolved
	encoded, err := json.Marshal(stream)
	if err != nil {
		return inbound
	}
	cp := *inbound
	cp.StreamSettings = string(encoded)
	return &cp
}
