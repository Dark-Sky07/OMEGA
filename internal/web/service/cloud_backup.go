package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

var (
	cloudBackupMu      sync.Mutex
	cloudBackupLastRun time.Time
)

// CloudBackupService handles manual and scheduled database backups to S3 / Cloudflare R2 / MinIO and WebDAV.
type CloudBackupService struct {
	settingService SettingService
	serverService  ServerService
}

// RunScheduledIfDue checks whether scheduled cloud backup is enabled and due to run.
func (s *CloudBackupService) RunScheduledIfDue() {
	enabled, err := s.settingService.GetCloudBackupEnable()
	if err != nil || !enabled {
		return
	}
	intervalHours, _ := s.settingService.GetCloudBackupIntervalHours()
	if intervalHours <= 0 {
		intervalHours = 12
	}
	interval := time.Duration(intervalHours) * time.Hour

	cloudBackupMu.Lock()
	if !cloudBackupLastRun.IsZero() && time.Since(cloudBackupLastRun) < interval {
		cloudBackupMu.Unlock()
		return
	}
	cloudBackupLastRun = time.Now()
	cloudBackupMu.Unlock()

	if key, err := s.BackupNow(); err != nil {
		logger.Warningf("scheduled cloud backup failed: %v", err)
	} else {
		logger.Infof("scheduled cloud backup uploaded: %s", key)
	}
}

// BackupNow creates a consistent database snapshot and uploads it to the configured cloud storage provider,
// then prunes older backup snapshots exceeding the retention limit.
func (s *CloudBackupService) BackupNow() (string, error) {
	provider, _ := s.settingService.GetCloudBackupProvider()
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = "s3"
	}

	endpoint, _ := s.settingService.GetCloudBackupEndpoint()
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", errors.New("cloud backup endpoint is required")
	}

	region, _ := s.settingService.GetCloudBackupRegion()
	region = strings.TrimSpace(region)
	if region == "" {
		region = "auto"
	}

	bucket, _ := s.settingService.GetCloudBackupBucket()
	bucket = strings.TrimSpace(bucket)
	prefix, _ := s.settingService.GetCloudBackupPrefix()
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	if prefix != "" {
		prefix += "/"
	}

	accessKey, _ := s.settingService.GetCloudBackupAccessKey()
	secretKey, _ := s.settingService.GetCloudBackupSecretKey()
	retention, _ := s.settingService.GetCloudBackupRetention()

	dbBytes, err := s.serverService.GetDb()
	if err != nil {
		return "", fmt.Errorf("read database snapshot: %w", err)
	}

	ext := ".db"
	if database.IsPostgres() {
		ext = ".dump"
	}
	fileName := fmt.Sprintf("x-ui-backup-%s%s", time.Now().UTC().Format("20060102-150405"), ext)
	objectKey := prefix + fileName

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := &http.Client{Timeout: 90 * time.Second}

	switch provider {
	case "webdav":
		if err := uploadWebDAV(ctx, client, endpoint, objectKey, accessKey, secretKey, dbBytes); err != nil {
			return "", err
		}
		if retention > 0 {
			if err := pruneWebDAVBackups(ctx, client, endpoint, prefix, accessKey, secretKey, retention); err != nil {
				logger.Warningf("cloud backup webdav retention cleanup warning: %v", err)
			}
		}
		return objectKey, nil
	case "s3", "r2", "minio":
		if bucket == "" {
			return "", errors.New("S3 bucket name is required")
		}
		if accessKey == "" || secretKey == "" {
			return "", errors.New("S3 access key and secret key are required")
		}
		if err := uploadS3Object(ctx, client, endpoint, region, bucket, objectKey, accessKey, secretKey, dbBytes); err != nil {
			return "", err
		}
		if retention > 0 {
			if err := pruneS3Backups(ctx, client, endpoint, region, bucket, prefix, accessKey, secretKey, retention); err != nil {
				logger.Warningf("cloud backup s3 retention cleanup warning: %v", err)
			}
		}
		return objectKey, nil
	default:
		return "", fmt.Errorf("unsupported cloud backup provider: %s", provider)
	}
}

func uploadWebDAV(ctx context.Context, client *http.Client, baseEndpoint, objectKey, username, password string, data []byte) error {
	targetURL, err := joinEndpointPath(baseEndpoint, objectKey)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, targetURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.ContentLength = int64(len(data))
	req.Header.Set("Content-Type", "application/octet-stream")
	if username != "" || password != "" {
		req.SetBasicAuth(username, password)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("webdav PUT failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

type webdavMultistatus struct {
	Responses []struct {
		Href string `xml:"href"`
	} `xml:"response"`
}

func pruneWebDAVBackups(ctx context.Context, client *http.Client, baseEndpoint, prefix, username, password string, keep int) error {
	dirURL, err := joinEndpointPath(baseEndpoint, prefix)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", dirURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Depth", "1")
	if username != "" || password != "" {
		req.SetBasicAuth(username, password)
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
		delURL, err := joinEndpointPath(baseEndpoint, prefix+name)
		if err != nil {
			continue
		}
		delReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, delURL, nil)
		if err != nil {
			continue
		}
		if username != "" || password != "" {
			delReq.SetBasicAuth(username, password)
		}
		if delResp, err := client.Do(delReq); err == nil {
			delResp.Body.Close()
		}
	}
	return nil
}

func uploadS3Object(ctx context.Context, client *http.Client, endpoint, region, bucket, objectKey, accessKey, secretKey string, data []byte) error {
	targetURL, err := joinEndpointPath(endpoint, bucket+"/"+objectKey)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, targetURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.ContentLength = int64(len(data))
	req.Header.Set("Content-Type", "application/octet-stream")
	signS3Request(req, data, region, accessKey, secretKey, time.Now().UTC())

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
