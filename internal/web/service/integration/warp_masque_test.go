package integration

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Dark-Sky07/phoenix/internal/database"
	"github.com/Dark-Sky07/phoenix/internal/database/model"
)

func withWarpMasqueAPIBase(t *testing.T, base string) {
	t.Helper()
	orig := warpMasqueAPIBase
	warpMasqueAPIBase = base
	t.Cleanup(func() { warpMasqueAPIBase = orig })
}

// Enrolling an ECDSA key replaces a device's WireGuard key, so MASQUE gets a device of
// its own; the returned warp block must pair the enrolled key and leave the stored
// WireGuard registration untouched.
func TestRegWarpMasqueEnrollsASeparateP256Device(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	if err := database.InitDB(filepath.Join(dbDir, "x-ui.db")); err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	if err := database.GetDB().Create(&model.Setting{Key: "warp", Value: `{"device_id":"old-device","access_token":"old-tok"}`}).Error; err != nil {
		t.Fatalf("seed warp: %v", err)
	}

	endpointKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate endpoint key: %v", err)
	}
	endpointDER, err := x509.MarshalPKIXPublicKey(&endpointKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal endpoint key: %v", err)
	}
	endpointPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: endpointDER}))

	var enrolledKey *ecdsa.PublicKey
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/reg":
			_, _ = w.Write([]byte(`{"id":"masque-device","token":"masque-token","account":{"license":""}}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/reg/masque-device":
			if got := r.Header.Get("Authorization"); got != "Bearer masque-token" {
				t.Errorf("enroll Authorization = %q, want the new device's token", got)
			}
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["key_type"] != "secp256r1" || body["tunnel_type"] != "masque" {
				t.Errorf("enroll body = %v, want a secp256r1 key for the masque tunnel", body)
			}
			der, err := base64.StdEncoding.DecodeString(body["key"])
			if err != nil {
				t.Errorf("enroll key is not base64: %v", err)
			}
			parsed, err := x509.ParsePKIXPublicKey(der)
			key, ok := parsed.(*ecdsa.PublicKey)
			if err != nil || !ok || key.Curve != elliptic.P256() {
				t.Errorf("enroll key is not a PKIX P-256 public key: %v", err)
			}
			enrolledKey = key
			resp, _ := json.Marshal(map[string]any{
				"id": "masque-device",
				"config": map[string]any{
					"interface": map[string]any{"addresses": map[string]any{"v4": "172.16.0.2", "v6": "2606:4700:110:8a36::2"}},
					"peers": []any{map[string]any{
						"public_key": endpointPEM,
						"endpoint":   map[string]any{"v4": "162.159.198.1:0", "v6": "[2606:4700:103::1]:0"},
					}},
				},
			})
			_, _ = w.Write(resp)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	withWarpMasqueAPIBase(t, srv.URL)

	out, err := (&WarpService{}).RegWarpMasque()
	if err != nil {
		t.Fatalf("RegWarpMasque: %v", err)
	}
	var result struct {
		PrivateKey string   `json:"privateKey"`
		PublicKey  string   `json:"publicKey"`
		Address    []string `json:"address"`
		Endpoint   string   `json:"endpoint"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if result.Endpoint != "162.159.198.1" || result.PublicKey != endpointPEM {
		t.Fatalf("endpoint = %q, publicKey = %q; want the enrolled peer's", result.Endpoint, result.PublicKey)
	}

	block, _ := pem.Decode([]byte(result.PrivateKey))
	if block == nil {
		t.Fatalf("privateKey %q is not PEM", result.PrivateKey)
	}
	private, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	ecPrivate, ok := private.(*ecdsa.PrivateKey)
	if err != nil || !ok || enrolledKey == nil || !ecPrivate.PublicKey.Equal(enrolledKey) {
		t.Fatal("the returned private key does not match the enrolled public key")
	}

	var stored model.Setting
	if err := database.GetDB().Where("key = ?", "warp").First(&stored).Error; err != nil {
		t.Fatalf("reload warp setting: %v", err)
	}
	var data map[string]string
	_ = json.Unmarshal([]byte(stored.Value), &data)
	if data["device_id"] != "old-device" {
		t.Fatalf("stored WireGuard registration = %v, want it untouched", data)
	}
}
