package agenthome

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/sts"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

type OSSConfig struct {
	Provider            string
	Endpoint            string
	Region              string
	Bucket              string
	BasePrefix          string
	AccessKeyID         string
	AccessKeySecret     string
	STSRoleARN          string
	STSDurationSeconds  int
	LocalDir            string
}

type STSCredentials struct {
	AccessKeyID     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
	SecurityToken   string `json:"security_token"`
	Expiration      string `json:"expiration"`

	Provider   string   `json:"provider"`
	Bucket     string   `json:"bucket"`
	Endpoint   string   `json:"endpoint"`
	Region     string   `json:"region"`
	BasePrefix string   `json:"base_prefix"`
	Prefixes   []string `json:"prefixes,omitempty"`
}

type OSSObjectStore interface {
	PutObject(ctx context.Context, key string, contentType string, body []byte) error
	GetObject(ctx context.Context, key string) ([]byte, error)
	ListObjects(ctx context.Context, prefix string, limit int) ([]string, error)
	Exists(ctx context.Context, key string) (bool, error)
}

type STSAssumer interface {
	AssumeRole(ctx context.Context, sessionName, policy string, durationSeconds int) (STSCredentials, error)
}

func JoinKey(basePrefix, key string) string {
	basePrefix = strings.Trim(strings.TrimSpace(basePrefix), "/")
	key = strings.TrimLeft(strings.TrimSpace(key), "/")
	if basePrefix == "" {
		return key
	}
	if key == "" {
		return basePrefix
	}
	return basePrefix + "/" + key
}

func NewOSSObjectStore(cfg OSSConfig) (OSSObjectStore, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "local":
		if strings.TrimSpace(cfg.LocalDir) == "" {
			return nil, errors.New("AIHUB_OSS_LOCAL_DIR is required when AIHUB_OSS_PROVIDER=local")
		}
		return localStore{root: cfg.LocalDir, basePrefix: cfg.BasePrefix}, nil
	case "aliyun":
		if cfg.Endpoint == "" || cfg.AccessKeyID == "" || cfg.AccessKeySecret == "" || cfg.Bucket == "" {
			return nil, errors.New("missing OSS config for aliyun provider")
		}
		client, err := oss.New(cfg.Endpoint, cfg.AccessKeyID, cfg.AccessKeySecret)
		if err != nil {
			return nil, err
		}
		bucket, err := client.Bucket(cfg.Bucket)
		if err != nil {
			return nil, err
		}
		return aliyunStore{bucket: bucket, basePrefix: cfg.BasePrefix}, nil
	default:
		return nil, errors.New("unsupported OSS provider (set AIHUB_OSS_PROVIDER=aliyun|local)")
	}
}

func NewSTSAssumer(cfg OSSConfig) (STSAssumer, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "local":
		return localSTS{cfg: cfg}, nil
	case "aliyun":
		if cfg.Region == "" {
			// Not strictly required by the SDK, but we need a stable way to pick endpoint.
			return nil, errors.New("AIHUB_OSS_REGION is required when AIHUB_OSS_PROVIDER=aliyun")
		}
		if cfg.AccessKeyID == "" || cfg.AccessKeySecret == "" || cfg.STSRoleARN == "" {
			return nil, errors.New("missing STS config (AIHUB_OSS_ACCESS_KEY_ID/SECRET + AIHUB_OSS_STS_ROLE_ARN)")
		}
		client, err := sts.NewClientWithAccessKey(cfg.Region, cfg.AccessKeyID, cfg.AccessKeySecret)
		if err != nil {
			return nil, err
		}
		return aliyunSTS{client: client, roleARN: cfg.STSRoleARN}, nil
	default:
		return nil, errors.New("unsupported OSS provider (set AIHUB_OSS_PROVIDER=aliyun|local)")
	}
}

type localStore struct {
	root       string
	basePrefix string
}

func (s localStore) PutObject(ctx context.Context, key string, contentType string, body []byte) error {
	_ = contentType
	fullKey := JoinKey(s.basePrefix, key)
	p := filepath.Join(s.root, filepath.FromSlash(fullKey))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	// Best-effort atomic write.
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func (s localStore) GetObject(ctx context.Context, key string) ([]byte, error) {
	_ = ctx
	fullKey := JoinKey(s.basePrefix, key)
	p := filepath.Join(s.root, filepath.FromSlash(fullKey))
	return os.ReadFile(p)
}

func (s localStore) ListObjects(ctx context.Context, prefix string, limit int) ([]string, error) {
	_ = ctx
	if limit <= 0 {
		limit = 100
	}
	fullPrefix := JoinKey(s.basePrefix, strings.TrimLeft(prefix, "/"))
	rootPath := filepath.Join(s.root, filepath.FromSlash(fullPrefix))

	var out []string
	walkRoot := filepath.Clean(rootPath)
	err := filepath.WalkDir(walkRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		out = append(out, key)
		if len(out) >= limit {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil && !errors.Is(err, filepath.SkipDir) {
		return nil, err
	}
	return out, nil
}

func (s localStore) Exists(ctx context.Context, key string) (bool, error) {
	_ = ctx
	fullKey := JoinKey(s.basePrefix, key)
	p := filepath.Join(s.root, filepath.FromSlash(fullKey))
	_, err := os.Stat(p)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

type aliyunStore struct {
	bucket     *oss.Bucket
	basePrefix string
}

func (s aliyunStore) PutObject(ctx context.Context, key string, contentType string, body []byte) error {
	fullKey := JoinKey(s.basePrefix, key)
	opts := []oss.Option{}
	if strings.TrimSpace(contentType) != "" {
		opts = append(opts, oss.ContentType(contentType))
	}
	return s.bucket.PutObject(fullKey, bytes.NewReader(body), opts...)
}

func (s aliyunStore) GetObject(ctx context.Context, key string) ([]byte, error) {
	_ = ctx
	fullKey := JoinKey(s.basePrefix, key)
	rc, err := s.bucket.GetObject(fullKey)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func (s aliyunStore) ListObjects(ctx context.Context, prefix string, limit int) ([]string, error) {
	_ = ctx
	if limit <= 0 {
		limit = 100
	}
	fullPrefix := JoinKey(s.basePrefix, strings.TrimLeft(prefix, "/"))
	res, err := s.bucket.ListObjects(oss.Prefix(fullPrefix), oss.MaxKeys(limit))
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(res.Objects))
	for _, o := range res.Objects {
		out = append(out, o.Key)
	}
	return out, nil
}

func (s aliyunStore) Exists(ctx context.Context, key string) (bool, error) {
	_ = ctx
	fullKey := JoinKey(s.basePrefix, key)
	return s.bucket.IsObjectExist(fullKey)
}

type localSTS struct {
	cfg OSSConfig
}

func (s localSTS) AssumeRole(ctx context.Context, sessionName, policy string, durationSeconds int) (STSCredentials, error) {
	_ = ctx
	_ = sessionName
	_ = policy
	if durationSeconds <= 0 {
		durationSeconds = s.cfg.STSDurationSeconds
	}
	exp := time.Now().Add(time.Duration(durationSeconds) * time.Second).UTC().Format(time.RFC3339)
	token, err := NewRandomChallenge()
	if err != nil {
		return STSCredentials{}, err
	}
	return STSCredentials{
		Provider:        "local",
		AccessKeyID:     "local",
		AccessKeySecret: "local",
		SecurityToken:   token,
		Expiration:      exp,
		Bucket:          s.cfg.Bucket,
		Endpoint:        s.cfg.Endpoint,
		Region:          s.cfg.Region,
		BasePrefix:      strings.Trim(strings.TrimSpace(s.cfg.BasePrefix), "/"),
	}, nil
}

type aliyunSTS struct {
	client  *sts.Client
	roleARN string
}

func (s aliyunSTS) AssumeRole(ctx context.Context, sessionName, policy string, durationSeconds int) (STSCredentials, error) {
	req := sts.CreateAssumeRoleRequest()
	req.Scheme = "https"
	req.RoleArn = s.roleARN
	req.RoleSessionName = sessionName
	req.Policy = policy
	req.DurationSeconds = requests.NewInteger(durationSeconds)

	// SDK doesn't take context; best-effort.
	resp, err := s.client.AssumeRole(req)
	if err != nil {
		return STSCredentials{}, err
	}
	if resp == nil || resp.Credentials.AccessKeyId == "" {
		return STSCredentials{}, errors.New("sts assume role returned empty credentials")
	}
	return STSCredentials{
		Provider:        "aliyun_sts",
		AccessKeyID:     resp.Credentials.AccessKeyId,
		AccessKeySecret: resp.Credentials.AccessKeySecret,
		SecurityToken:   resp.Credentials.SecurityToken,
		Expiration:      resp.Credentials.Expiration,
	}, nil
}

func BuildOSSPolicy(bucket string, allowListPrefixes, allowReadPrefixes, allowWritePrefixes []string) (string, error) {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return "", errors.New("missing bucket")
	}

	type statement struct {
		Effect    string                 `json:"Effect"`
		Action    []string               `json:"Action"`
		Resource  []string               `json:"Resource"`
		Condition map[string]map[string][]string `json:"Condition,omitempty"`
	}

	dedupe := func(in []string) []string {
		out := make([]string, 0, len(in))
		seen := map[string]struct{}{}
		for _, p := range in {
			p = strings.TrimLeft(strings.TrimSpace(p), "/")
			if p == "" {
				continue
			}
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			out = append(out, p)
		}
		return out
	}

	allowListPrefixes = dedupe(allowListPrefixes)
	allowReadPrefixes = dedupe(allowReadPrefixes)
	allowWritePrefixes = dedupe(allowWritePrefixes)

	var stmts []statement

	if len(allowListPrefixes) > 0 {
		// For OSS ListObjects, the bucket resource is required and prefix constraints are set via Condition.
		prefixPatterns := make([]string, 0, len(allowListPrefixes)*2)
		for _, p := range allowListPrefixes {
			prefixPatterns = append(prefixPatterns, p)
			if !strings.HasSuffix(p, "*") {
				prefixPatterns = append(prefixPatterns, p+"*")
			}
		}
		stmts = append(stmts, statement{
			Effect:   "Allow",
			Action:   []string{"oss:ListObjects"},
			Resource: []string{fmt.Sprintf("acs:oss:*:*:%s", bucket)},
			Condition: map[string]map[string][]string{
				"StringLike": {
					"oss:Prefix": prefixPatterns,
				},
			},
		})
	}

	if len(allowReadPrefixes) > 0 {
		resources := make([]string, 0, len(allowReadPrefixes))
		for _, p := range allowReadPrefixes {
			if strings.HasSuffix(p, "*") {
				resources = append(resources, fmt.Sprintf("acs:oss:*:*:%s/%s", bucket, p))
				continue
			}
			resources = append(resources, fmt.Sprintf("acs:oss:*:*:%s/%s*", bucket, p))
		}
		stmts = append(stmts, statement{
			Effect:   "Allow",
			Action:   []string{"oss:GetObject"},
			Resource: resources,
		})
	}

	if len(allowWritePrefixes) > 0 {
		resources := make([]string, 0, len(allowWritePrefixes))
		for _, p := range allowWritePrefixes {
			if strings.HasSuffix(p, "*") {
				resources = append(resources, fmt.Sprintf("acs:oss:*:*:%s/%s", bucket, p))
				continue
			}
			// Treat a trailing "/" as a prefix write.
			if strings.HasSuffix(p, "/") {
				resources = append(resources, fmt.Sprintf("acs:oss:*:*:%s/%s*", bucket, p))
				continue
			}
			resources = append(resources, fmt.Sprintf("acs:oss:*:*:%s/%s", bucket, p))
		}
		stmts = append(stmts, statement{
			Effect:   "Allow",
			Action:   []string{"oss:PutObject"},
			Resource: resources,
		})
	}

	policy := map[string]any{
		"Version":   "1",
		"Statement": stmts,
	}
	b, err := json.Marshal(policy)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
