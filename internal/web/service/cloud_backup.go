package service

import (
	"bytes"
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

// CloudBackupService uploads periodic database snapshots to S3-compatible
// object storage (AWS S3, Cloudflare R2, MinIO, Backblaze B2), WebDAV
// servers (Nextcloud, ownCloud, Hetzner Storage Box), or Google Drive (v3 API)
// and enforces retention.
type CloudBackupService struct {
	settingService SettingService
}

// CloudBackupOverride carries optional in-memory settings from the UI so
// clicking "Backup Now" works immediately even before clicking the top Save
// button.
type CloudBackupOverride struct {
	Provider  string `json:"cloudBackupProvider" form:"cloudBackupProvider"`
	Endpoint  string `json:"cloudBackupEndpoint" form:"cloudBackupEndpoint"`
	Region    string `json:"cloudBackupRegion" form:"cloudBackupRegion"`
	Bucket    string `json:"cloudBackupBucket" form:"cloudBackupBucket"`
	Prefix    string `json:"cloudBackupPrefix" form:"cloudBackupPrefix"`
	AccessKey string `json:"cloudBackupAccessKey" form:"cloudBackupAccessKey"`
	SecretKey string `json:"cloudBackupSecretKey" form:"cloudBackupSecretKey"`
	Retention *int   `json:"cloudBackupRetention" form:"cloudBackupRetention"`
}

var (
	cloudBackupMu      sync.Mutex
	lastCloudBackupRun time.Time
)

var (
	googleOAuthTokenURL = "https://oauth2.googleapis.com/token"
	googleDriveAPIBase  = "https://www.googleapis.com"
)

// RunScheduledIfDue is called by the periodic cron job and triggers a backup
// when the configured interval has elapsed.
func (s *CloudBackupService) RunScheduledIfDue() {
	s.RunIfDue()
}

// RunIfDue is called by the periodic cron job and triggers a backup when the
// configured interval has elapsed.
func (s *CloudBackupService) RunIfDue() {
	enabled, err := s.settingService.GetCloudBackupEnable()
	if err != nil || !enabled {
		return
	}
	intervalHours, err := s.settingService.GetCloudBackupIntervalHours()
	if err != nil || intervalHours <= 0 {
		intervalHours = 12
	}
	cloudBackupMu.Lock()
	if !lastCloudBackupRun.IsZero() && time.Since(lastCloudBackupRun) < time.Duration(intervalHours)*time.Hour {
		cloudBackupMu.Unlock()
		return
	}
	lastCloudBackupRun = time.Now()
	cloudBackupMu.Unlock()

	if _, err := s.BackupNow(); err != nil {
		logger.Warning("cloud backup failed:", err)
	}
}

// BackupNow creates a fresh database snapshot and uploads it to the configured
// cloud storage provider, then prunes older backups beyond the retention limit.
func (s *CloudBackupService) BackupNow() (string, error) {
	return s.BackupNowWithOverride(nil)
}

// BackupNowWithOverride runs BackupNow with optional form overrides merged over
// the persisted settings.
func (s *CloudBackupService) BackupNowWithOverride(override *CloudBackupOverride) (string, error) {
	provider, _ := s.settingService.GetCloudBackupProvider()
	endpoint, _ := s.settingService.GetCloudBackupEndpoint()
	region, _ := s.settingService.GetCloudBackupRegion()
	bucket, _ := s.settingService.GetCloudBackupBucket()
	prefix, _ := s.settingService.GetCloudBackupPrefix()
	accessKey, _ := s.settingService.GetCloudBackupAccessKey()
	secretKey, _ := s.settingService.GetCloudBackupSecretKey()
	retention, _ := s.settingService.GetCloudBackupRetention()

	if override != nil {
		if v := strings.TrimSpace(override.Provider); v != "" {
			provider = v
		}
		if v := strings.TrimSpace(override.Endpoint); v != "" {
			endpoint = v
		}
		if v := strings.TrimSpace(override.Region); v != "" {
			region = v
		}
		if v := strings.TrimSpace(override.Bucket); v != "" {
			bucket = v
		}
		if v := strings.TrimSpace(override.Prefix); v != "" {
			prefix = v
		}
		if v := strings.TrimSpace(override.AccessKey); v != "" {
			accessKey = v
		}
		if v := strings.TrimSpace(override.SecretKey); v != "" {
			secretKey = v
		}
		if override.Retention != nil && *override.Retention >= 0 {
			retention = *override.Retention
		}
	}

	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = "s3"
	}
	endpoint = strings.TrimSpace(endpoint)
	region = strings.TrimSpace(region)
	if region == "" {
		region = "auto"
	}
	bucket = strings.TrimSpace(bucket)
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	if prefix != "" {
		prefix += "/"
	}
	accessKey = strings.TrimSpace(accessKey)
	secretKey = strings.TrimSpace(secretKey)

	if provider != "gdrive" && endpoint == "" {
		return "", fmt.Errorf("cloud backup endpoint is required")
	}

	payload, ext, err := buildDatabaseBackupPayload()
	if err != nil {
		return "", err
	}

	ts := time.Now().UTC().Format("20060102-150405")
	fileName := fmt.Sprintf("x-ui-backup-%s%s", ts, ext)
	objectKey := prefix + fileName

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := &http.Client{Timeout: 90 * time.Second}

	switch provider {
	case "webdav":
		if err := uploadWebDAV(ctx, client, endpoint, objectKey, accessKey, secretKey, payload); err != nil {
			return "", err
		}
		if retention > 0 {
			_ = pruneWebDAVBackups(ctx, client, endpoint, prefix, accessKey, secretKey, retention)
		}
	case "s3":
		if bucket == "" {
			return "", fmt.Errorf("cloud backup S3 bucket is required")
		}
		if accessKey == "" || secretKey == "" {
			return "", fmt.Errorf("cloud backup S3 access key and secret key are required")
		}
		if err := uploadS3(ctx, client, endpoint, region, bucket, objectKey, accessKey, secretKey, payload); err != nil {
			return "", err
		}
		if retention > 0 {
			_ = pruneS3Backups(ctx, client, endpoint, region, bucket, prefix, accessKey, secretKey, retention)
		}
	case "gdrive", "google_drive", "googledrive":
		driveName := fileName
		if pfx := strings.Trim(prefix, "/"); pfx != "" && pfx != "omega-backups" {
			driveName = strings.ReplaceAll(pfx, "/", "-") + "-" + fileName
		}
		folderID := extractGoogleDriveFolderID(bucket)
		uploadedName, err := uploadGoogleDrive(ctx, client, endpoint, accessKey, secretKey, folderID, driveName, payload, retention)
		if err != nil {
			return "", err
		}
		objectKey = uploadedName
	default:
		return "", fmt.Errorf("unsupported cloud backup provider: %s", provider)
	}

	cloudBackupMu.Lock()
	lastCloudBackupRun = time.Now()
	cloudBackupMu.Unlock()

	logger.Infof("cloud backup uploaded (%s): %s (%d bytes)", provider, objectKey, len(payload))
	return objectKey, nil
}

func buildDatabaseBackupPayload() ([]byte, string, error) {
	if database.IsPostgres() {
		var srv ServerService
		b, err := srv.GetDb()
		return b, ".dump", err
	}
	if err := database.Checkpoint(); err != nil {
		logger.Warning("SQLite checkpoint before cloud backup:", err)
	}
	b, err := os.ReadFile(config.GetDBPath())
	return b, ".db", err
}

// --- Google Drive v3 implementation ---

type googleServiceAccountKey struct {
	Type         string `json:"type"`
	ClientEmail  string `json:"client_email"`
	PrivateKey   string `json:"private_key"`
	TokenURI     string `json:"token_uri"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
}

func extractGoogleDriveFolderID(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		if id := u.Query().Get("id"); id != "" {
			return strings.TrimSpace(id)
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		for i := 0; i+1 < len(parts); i++ {
			if parts[i] == "folders" && parts[i+1] != "" {
				return strings.TrimSpace(parts[i+1])
			}
		}
	}
	return s
}

func uploadGoogleDrive(ctx context.Context, client *http.Client, clientSecretOrEndpoint, clientID, credentialsOrRefreshToken, folderID, fileName string, payload []byte, retention int) (string, error) {
	if strings.TrimSpace(credentialsOrRefreshToken) == "" {
		return "", fmt.Errorf("Google Drive Service Account JSON or OAuth2 Refresh Token is required")
	}
	apiBase := googleDriveAPIBase
	tokenURL := googleOAuthTokenURL
	if strings.HasPrefix(clientSecretOrEndpoint, "http://") || strings.HasPrefix(clientSecretOrEndpoint, "https://") {
		apiBase = strings.TrimRight(clientSecretOrEndpoint, "/")
		tokenURL = apiBase + "/token"
	}

	accessToken, err := obtainGoogleDriveAccessToken(ctx, client, tokenURL, clientID, clientSecretOrEndpoint, credentialsOrRefreshToken)
	if err != nil {
		return "", err
	}

	meta := map[string]any{
		"name": fileName,
	}
	if folderID != "" {
		meta["parents"] = []string{folderID}
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return "", err
	}

	boundary := fmt.Sprintf("omega_gdrive_%d", time.Now().UnixNano())
	var body bytes.Buffer
	fmt.Fprintf(&body, "--%s\r\nContent-Type: application/json; charset=UTF-8\r\n\r\n%s\r\n", boundary, string(metaJSON))
	fmt.Fprintf(&body, "--%s\r\nContent-Type: application/octet-stream\r\n\r\n", boundary)
	body.Write(payload)
	fmt.Fprintf(&body, "\r\n--%s--\r\n", boundary)

	uploadURL := apiBase + "/upload/drive/v3/files?uploadType=multipart&supportsAllDrives=true&fields=id,name"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "multipart/related; boundary="+boundary)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Google Drive upload request failed: %w", err)
	}
	defer resp.Body.Close()
	respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("Google Drive upload failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(respBytes)))
	}

	if retention > 0 {
		_ = pruneGoogleDriveBackups(ctx, client, apiBase, accessToken, folderID, retention)
	}
	if folderID != "" {
		return folderID + "/" + fileName, nil
	}
	return fileName, nil
}

func obtainGoogleDriveAccessToken(ctx context.Context, client *http.Client, defaultTokenURL, clientID, clientSecret, rawCreds string) (string, error) {
	trimmed := strings.TrimSpace(rawCreds)
	if strings.HasPrefix(trimmed, "ya29.") {
		return trimmed, nil
	}

	if strings.HasPrefix(trimmed, "{") {
		var key googleServiceAccountKey
		if err := json.Unmarshal([]byte(trimmed), &key); err != nil {
			return "", fmt.Errorf("invalid Google Drive credentials JSON: %w", err)
		}
		tokenURI := strings.TrimSpace(key.TokenURI)
		if tokenURI == "" || defaultTokenURL != googleOAuthTokenURL {
			tokenURI = defaultTokenURL
		}
		if key.PrivateKey != "" && key.ClientEmail != "" {
			jwtAssertion, err := buildGoogleServiceAccountJWT(key.ClientEmail, key.PrivateKey, tokenURI, time.Now().UTC())
			if err != nil {
				return "", err
			}
			form := url.Values{}
			form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
			form.Set("assertion", jwtAssertion)
			return exchangeGoogleOAuthForm(ctx, client, tokenURI, form)
		}
		if key.RefreshToken != "" {
			cid := firstNonEmptyString(key.ClientID, clientID)
			csec := firstNonEmptyString(key.ClientSecret, clientSecret)
			if cid == "" || csec == "" {
				return "", fmt.Errorf("Google Drive OAuth2 JSON requires client_id and client_secret")
			}
			form := url.Values{}
			form.Set("grant_type", "refresh_token")
			form.Set("client_id", cid)
			form.Set("client_secret", csec)
			form.Set("refresh_token", key.RefreshToken)
			return exchangeGoogleOAuthForm(ctx, client, tokenURI, form)
		}
		return "", fmt.Errorf("Google Drive JSON must contain either service_account (client_email + private_key) or OAuth2 (client_id + client_secret + refresh_token)")
	}

	if strings.TrimSpace(clientID) == "" || strings.TrimSpace(clientSecret) == "" {
		return "", fmt.Errorf("Google Drive OAuth2 Refresh Token mode requires Client ID and Client Secret (or paste a full Service Account JSON key)")
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", strings.TrimSpace(clientID))
	form.Set("client_secret", strings.TrimSpace(clientSecret))
	form.Set("refresh_token", trimmed)
	return exchangeGoogleOAuthForm(ctx, client, defaultTokenURL, form)
}

func exchangeGoogleOAuthForm(ctx context.Context, client *http.Client, tokenURL string, form url.Values) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Google OAuth2 token exchange failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("Google OAuth2 token exchange failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil || strings.TrimSpace(tokenResp.AccessToken) == "" {
		return "", fmt.Errorf("Google OAuth2 token response missing access_token")
	}
	return tokenResp.AccessToken, nil
}

func buildGoogleServiceAccountJWT(clientEmail, privateKeyPEM, tokenURI string, now time.Time) (string, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return "", fmt.Errorf("invalid Google Service Account private_key PEM")
	}
	var rsaKey *rsa.PrivateKey
	if parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if k, ok := parsed.(*rsa.PrivateKey); ok {
			rsaKey = k
		}
	}
	if rsaKey == nil {
		if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
			rsaKey = k
		}
	}
	if rsaKey == nil {
		return "", fmt.Errorf("Google Service Account private_key is not a valid RSA key")
	}

	headerJSON := `{"alg":"RS256","typ":"JWT"}`
	claims := map[string]any{
		"iss":   clientEmail,
		"scope": "https://www.googleapis.com/auth/drive",
		"aud":   tokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(55 * time.Minute).Unix(),
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding
	signingInput := enc.EncodeToString([]byte(headerJSON)) + "." + enc.EncodeToString(claimsJSON)
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, rsaKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign Google Service Account JWT: %w", err)
	}
	return signingInput + "." + enc.EncodeToString(sig), nil
}

func pruneGoogleDriveBackups(ctx context.Context, client *http.Client, apiBase, accessToken, folderID string, keep int) error {
	q := "trashed = false and name contains 'x-ui-backup-'"
	if folderID != "" {
		escapedFolder := strings.ReplaceAll(folderID, "'", "\\'")
		q += fmt.Sprintf(" and '%s' in parents", escapedFolder)
	}
	u, err := url.Parse(apiBase + "/drive/v3/files")
	if err != nil {
		return err
	}
	params := u.Query()
	params.Set("q", q)
	params.Set("pageSize", "200")
	params.Set("fields", "files(id,name,createdTime)")
	params.Set("supportsAllDrives", "true")
	params.Set("includeItemsFromAllDrives", "true")
	u.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	var listRes struct {
		Files []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			CreatedTime string `json:"createdTime"`
		} `json:"files"`
	}
	if err := json.Unmarshal(body, &listRes); err != nil {
		return err
	}
	type driveBackupItem struct {
		id   string
		name string
	}
	var backups []driveBackupItem
	for _, f := range listRes.Files {
		if strings.Contains(f.Name, "x-ui-backup-") && (strings.HasSuffix(f.Name, ".db") || strings.HasSuffix(f.Name, ".dump")) {
			backups = append(backups, driveBackupItem{id: f.ID, name: f.Name})
		}
	}
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].name < backups[j].name
	})
	if len(backups) <= keep {
		return nil
	}
	for _, old := range backups[:len(backups)-keep] {
		delURL := fmt.Sprintf("%s/drive/v3/files/%s?supportsAllDrives=true", apiBase, url.PathEscape(old.id))
		delReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, delURL, nil)
		if err != nil {
			continue
		}
		delReq.Header.Set("Authorization", "Bearer "+accessToken)
		if delResp, err := client.Do(delReq); err == nil {
			delResp.Body.Close()
		}
	}
	return nil
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return ""
}

// --- WebDAV implementation ---

func uploadWebDAV(ctx context.Context, client *http.Client, endpoint, objectKey, user, pass string, payload []byte) error {
	targetURL, err := joinEndpointPath(endpoint, objectKey)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, targetURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.ContentLength = int64(len(payload))
	req.Header.Set("Content-Type", "application/octet-stream")
	if user != "" || pass != "" {
		req.SetBasicAuth(user, pass)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("WebDAV PUT failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

type webdavMultistatus struct {
	Responses []struct {
		Href string `xml:"href"`
	} `xml:"response"`
}

func pruneWebDAVBackups(ctx context.Context, client *http.Client, endpoint, prefix, user, pass string, keep int) error {
	dirURL, err := joinEndpointPath(endpoint, prefix)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", dirURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Depth", "1")
	if user != "" || pass != "" {
		req.SetBasicAuth(user, pass)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	var ms webdavMultistatus
	if err := xml.Unmarshal(body, &ms); err != nil {
		return err
	}
	var names []string
	for _, r := range ms.Responses {
		base := path.Base(strings.TrimRight(r.Href, "/"))
		if strings.HasPrefix(base, "x-ui-backup-") && (strings.HasSuffix(base, ".db") || strings.HasSuffix(base, ".dump")) {
			names = append(names, base)
		}
	}
	sort.Strings(names)
	if len(names) <= keep {
		return nil
	}
	toDelete := names[:len(names)-keep]
	for _, name := range toDelete {
		delURL, err := joinEndpointPath(endpoint, prefix+name)
		if err != nil {
			continue
		}
		delReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, delURL, nil)
		if err != nil {
			continue
		}
		if user != "" || pass != "" {
			delReq.SetBasicAuth(user, pass)
		}
		if delResp, err := client.Do(delReq); err == nil {
			delResp.Body.Close()
		}
	}
	return nil
}

// --- S3 SigV4 implementation (path-style, zero external SDK needed) ---

func uploadS3Object(ctx context.Context, client *http.Client, endpoint, region, bucket, objectKey, accessKey, secretKey string, payload []byte) error {
	return uploadS3(ctx, client, endpoint, region, bucket, objectKey, accessKey, secretKey, payload)
}

func uploadS3(ctx context.Context, client *http.Client, endpoint, region, bucket, objectKey, accessKey, secretKey string, payload []byte) error {
	targetURL, err := joinEndpointPath(endpoint, bucket+"/"+objectKey)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, targetURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.ContentLength = int64(len(payload))
	req.Header.Set("Content-Type", "application/octet-stream")
	signS3Request(req, payload, region, accessKey, secretKey, time.Now().UTC())

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("S3 PUT failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

type s3ListBucketResult struct {
	Contents []struct {
		Key string `xml:"Key"`
	} `xml:"Contents"`
}

func pruneS3Backups(ctx context.Context, client *http.Client, endpoint, region, bucket, prefix, accessKey, secretKey string, keep int) error {
	bucketURL, err := joinEndpointPath(endpoint, bucket)
	if err != nil {
		return err
	}
	u, err := url.Parse(bucketURL)
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("list-type", "2")
	if prefix != "" {
		q.Set("prefix", prefix)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	signS3Request(req, nil, region, accessKey, secretKey, time.Now().UTC())
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	var listRes s3ListBucketResult
	if err := xml.Unmarshal(body, &listRes); err != nil {
		return err
	}
	var keys []string
	for _, c := range listRes.Contents {
		base := path.Base(c.Key)
		if strings.HasPrefix(base, "x-ui-backup-") && (strings.HasSuffix(base, ".db") || strings.HasSuffix(base, ".dump")) {
			keys = append(keys, c.Key)
		}
	}
	sort.Strings(keys)
	if len(keys) <= keep {
		return nil
	}
	toDelete := keys[:len(keys)-keep]
	for _, k := range toDelete {
		delURL, err := joinEndpointPath(endpoint, bucket+"/"+k)
		if err != nil {
			continue
		}
		delReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, delURL, nil)
		if err != nil {
			continue
		}
		signS3Request(delReq, nil, region, accessKey, secretKey, time.Now().UTC())
		if delResp, err := client.Do(delReq); err == nil {
			delResp.Body.Close()
		}
	}
	return nil
}

func joinEndpointPath(baseEndpoint, relPath string) (string, error) {
	raw := strings.TrimSpace(baseEndpoint)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	cleanRel := strings.TrimLeft(relPath, "/")
	basePath := strings.TrimRight(u.Path, "/")
	if cleanRel != "" {
		u.Path = basePath + "/" + cleanRel
	} else if basePath == "" {
		u.Path = "/"
	} else {
		u.Path = basePath
	}
	return u.String(), nil
}

func signS3Request(req *http.Request, payload []byte, region, accessKey, secretKey string, now time.Time) {
	if region == "" {
		region = "auto"
	}
	amzDate := now.UTC().Format("20060102T150405Z")
	dateStamp := now.UTC().Format("20060102")
	sum := sha256.Sum256(payload)
	payloadHash := hex.EncodeToString(sum[:])

	req.Header.Set("Host", req.URL.Host)
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHash)

	canonicalURI := req.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	canonicalQuery := canonicalS3Query(req.URL.Query())
	canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n",
		strings.ToLower(req.URL.Host), payloadHash, amzDate)
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"

	canonicalReq := strings.Join([]string{
		req.Method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	crSum := sha256.Sum256([]byte(canonicalReq))
	scope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, region)
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		hex.EncodeToString(crSum[:]),
	}, "\n")

	kDate := hmacSHA256([]byte("AWS4"+secretKey), dateStamp)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, "s3")
	kSigning := hmacSHA256(kService, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))

	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, scope, signedHeaders, signature)
	req.Header.Set("Authorization", authHeader)
}

func canonicalS3Query(q url.Values) string {
	if len(q) == 0 {
		return ""
	}
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vals := append([]string(nil), q[k]...)
		sort.Strings(vals)
		ek := s3URIEncode(k)
		for _, v := range vals {
			parts = append(parts, ek+"="+s3URIEncode(v))
		}
	}
	return strings.Join(parts, "&")
}

func s3URIEncode(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(data))
	return h.Sum(nil)
}
