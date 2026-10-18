package sub

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// XDRIVE has no share-link format: a link would either drop the storage secrets both
// ends need or print them into every QR code, so only the JSON subscription carries it.
func TestGetLink_SkipsXdriveStreams(t *testing.T) {
	stream := `{"network":"xdrive","security":"none",
		"xdriveSettings":{"service":"Google Drive","remoteFolder":"xray-tunnel","secrets":["id","secret","token"]}}`
	inbound := &model.Inbound{
		Protocol:       model.VLESS,
		Port:           443,
		Settings:       `{"clients":[{"id":"11111111-1111-1111-1111-111111111111","email":"user"}],"decryption":"none"}`,
		StreamSettings: stream,
	}

	if link := (&SubService{}).GetLink(inbound, "user"); link != "" {
		t.Fatalf("GetLink = %q, want no link for an XDRIVE stream", link)
	}
}
