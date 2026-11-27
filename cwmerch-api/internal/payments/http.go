package payments

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var client = &http.Client{Timeout: 20 * time.Second}

// APIError is a provider refusal, kept with its body for the logs.
type APIError struct {
	Provider string
	Status   int
	Body     string
}

func (e *APIError) Error() string {
	body := e.Body
	if len(body) > 300 {
		body = body[:300]
	}
	return fmt.Sprintf("%s answered %d: %s", e.Provider, e.Status, body)
}

func doJSON(ctx context.Context, provider, method, url string, headers map[string]string, in any, out any) (int, error) {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(raw)
	}
	return do(ctx, provider, method, url, headers, body, "application/json", out)
}

func do(ctx context.Context, provider, method, url string, headers map[string]string, body io.Reader, contentType string, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("%s is not reachable: %w", provider, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return resp.StatusCode, &APIError{Provider: provider, Status: resp.StatusCode, Body: string(raw)}
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp.StatusCode, fmt.Errorf("%s answered unreadable JSON: %w", provider, err)
		}
	}
	return resp.StatusCode, nil
}

// verifyTimestampedHMAC checks a "t=<unix>,v1=<hex>" signature over
// "<t>.<payload>" (Stripe's and Flouci's scheme), within tolerance.
func verifyTimestampedHMAC(header, secret string, payload []byte, tolerance time.Duration) error {
	if header == "" || secret == "" {
		return ErrBadSignature
	}
	var ts string
	sigs := []string{}
	for _, part := range strings.Split(header, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			ts = kv[1]
		case "v1":
			sigs = append(sigs, kv[1])
		}
	}
	n, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || len(sigs) == 0 {
		return ErrBadSignature
	}
	if age := time.Since(time.Unix(n, 0)); age > tolerance || age < -tolerance {
		return ErrBadSignature
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(payload)
	expected := mac.Sum(nil)
	for _, s := range sigs {
		got, err := hex.DecodeString(s)
		if err == nil && hmac.Equal(got, expected) {
			return nil
		}
	}
	return ErrBadSignature
}

func signTimestamped(secret string, ts int64, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10) + "."))
	mac.Write(payload)
	return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

func fmtFloat(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func orderIDFrom(s string) uint64 {
	s = strings.TrimPrefix(strings.TrimSpace(s), "order-")
	n, _ := strconv.ParseUint(s, 10, 64)
	return n
}
