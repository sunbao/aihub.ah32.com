package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"aihub/internal/agenthome"
)

type keySet struct {
	Keys []struct {
		KeyID     string `json:"key_id"`
		Alg       string `json:"alg"`
		PublicKey string `json:"public_key"`
	} `json:"keys"`
}

func main() {
	var (
		filePath = flag.String("file", "", "Path to JSON file to verify ('-' for stdin)")
		keysURL  = flag.String("keys-url", "", "Platform signing keyset URL (e.g. http://localhost:8080/v1/platform/signing-keys)")
		keysFile = flag.String("keys-file", "", "Path to keyset JSON file (same shape as /v1/platform/signing-keys)")
	)
	flag.Parse()

	if strings.TrimSpace(*filePath) == "" {
		fmt.Fprintln(os.Stderr, "missing -file")
		os.Exit(2)
	}
	if strings.TrimSpace(*keysURL) == "" && strings.TrimSpace(*keysFile) == "" {
		fmt.Fprintln(os.Stderr, "missing -keys-url or -keys-file")
		os.Exit(2)
	}

	obj, err := readJSONFile(*filePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read object:", err)
		os.Exit(1)
	}

	ks, err := loadKeySet(*keysURL, *keysFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "load keyset:", err)
		os.Exit(1)
	}

	if err := verifyCertifiedObject(obj, ks); err != nil {
		fmt.Fprintln(os.Stderr, "verify failed:", err)
		os.Exit(1)
	}
	fmt.Println("OK")
}

func readJSONFile(path string) (map[string]any, error) {
	var b []byte
	var err error
	if path == "-" {
		b, err = io.ReadAll(os.Stdin)
	} else {
		b, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}
	var obj map[string]any
	if err := json.Unmarshal(b, &obj); err != nil {
		return nil, err
	}
	return obj, nil
}

func loadKeySet(url string, path string) (keySet, error) {
	var b []byte
	var err error
	if strings.TrimSpace(path) != "" {
		b, err = os.ReadFile(path)
		if err != nil {
			return keySet{}, err
		}
	} else {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return keySet{}, err
		}
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return keySet{}, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return keySet{}, fmt.Errorf("http %d", resp.StatusCode)
		}
		b, err = io.ReadAll(resp.Body)
		if err != nil {
			return keySet{}, err
		}
	}

	var ks keySet
	if err := json.Unmarshal(b, &ks); err != nil {
		return keySet{}, err
	}
	return ks, nil
}

func verifyCertifiedObject(obj map[string]any, ks keySet) error {
	certAny, ok := obj["cert"]
	if !ok || certAny == nil {
		return errors.New("missing cert")
	}
	certMap, ok := certAny.(map[string]any)
	if !ok {
		return errors.New("invalid cert shape")
	}

	getString := func(k string) string {
		v, _ := certMap[k].(string)
		return strings.TrimSpace(v)
	}
	keyID := getString("key_id")
	sigB64 := getString("signature")
	if keyID == "" || sigB64 == "" {
		return errors.New("missing cert.key_id or cert.signature")
	}

	var pubB64 string
	for _, k := range ks.Keys {
		if strings.TrimSpace(k.KeyID) == keyID {
			pubB64 = strings.TrimSpace(k.PublicKey)
			break
		}
	}
	if pubB64 == "" {
		return fmt.Errorf("unknown key_id: %s", keyID)
	}
	pubRaw, err := base64.StdEncoding.DecodeString(pubB64)
	if err != nil {
		return fmt.Errorf("invalid public_key encoding: %w", err)
	}

	issuedAt := getString("issued_at")
	expiresAt := getString("expires_at")
	if expiresAt != "" {
		if t, err := time.Parse(time.RFC3339, expiresAt); err == nil && time.Now().After(t) {
			return errors.New("cert expired")
		}
	}
	if issuedAt != "" {
		if _, err := time.Parse(time.RFC3339, issuedAt); err != nil {
			return errors.New("invalid cert.issued_at")
		}
	}

	signable := make(map[string]any, len(obj))
	for k, v := range obj {
		if k == "cert" {
			continue
		}
		signable[k] = v
	}
	canonical, err := agenthome.CanonicalJSON(signable)
	if err != nil {
		return err
	}

	okSig, err := agenthome.VerifyEd25519Base64(ed25519.PublicKey(pubRaw), canonical, sigB64)
	if err != nil {
		return err
	}
	if !okSig {
		return errors.New("signature verification failed")
	}
	return nil
}
