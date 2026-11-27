package payments

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestMinorUnits(t *testing.T) {
	cases := []struct {
		amount   float64
		currency string
		want     int64
	}{{12.5, "EUR", 1250}, {10.255, "TND", 10255}, {1500, "XOF", 1500}, {0.1 + 0.2, "USD", 30}}
	for _, c := range cases {
		if got := MinorUnits(c.amount, c.currency); got != c.want {
			t.Errorf("MinorUnits(%v, %s) = %d, want %d", c.amount, c.currency, got, c.want)
		}
	}
	if FromMinor(10255, "TND") != 10.255 {
		t.Error("FromMinor TND")
	}
}

func TestTimestampedSignature(t *testing.T) {
	body := []byte(`{"id":"evt_1"}`)
	header := signTimestamped("whsec_x", time.Now().Unix(), body)
	if err := verifyTimestampedHMAC(header, "whsec_x", body, 5*time.Minute); err != nil {
		t.Fatalf("valid signature refused: %v", err)
	}
	if err := verifyTimestampedHMAC(header, "other", body, 5*time.Minute); err == nil {
		t.Fatal("wrong secret accepted")
	}
	if err := verifyTimestampedHMAC(header, "whsec_x", []byte(`{"id":"evt_2"}`), 5*time.Minute); err == nil {
		t.Fatal("tampered body accepted")
	}
	old := signTimestamped("whsec_x", time.Now().Add(-time.Hour).Unix(), body)
	if err := verifyTimestampedHMAC(old, "whsec_x", body, 5*time.Minute); err == nil {
		t.Fatal("replayed (old) signature accepted")
	}
}

func TestStripeCreateStatusWebhook(t *testing.T) {
	var form url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk_test_1" {
			w.WriteHeader(401)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/payment_intents":
			raw, _ := io.ReadAll(r.Body)
			form, _ = url.ParseQuery(string(raw))
			_, _ = w.Write([]byte(`{"id":"pi_1","client_secret":"pi_1_secret","status":"requires_payment_method"}`))
		case r.URL.Path == "/payment_intents/pi_1":
			_, _ = w.Write([]byte(`{"id":"pi_1","status":"succeeded","amount_received":10260,"currency":"tnd","latest_charge":"ch_9","metadata":{"order_id":"42"}}`))
		}
	}))
	defer srv.Close()
	s := Stripe{BaseURL: srv.URL}
	cfg := Config{Settings: map[string]any{"publishableKey": "pk_test_1"}, Secrets: map[string]string{"secretKey": "sk_test_1", "webhookSecret": "whsec_1"}}
	sess, err := s.CreatePayment(context.Background(), cfg, Request{OrderID: 42, OrderKey: "k", Amount: 10.256, Currency: "TND"})
	if err != nil || sess.ClientSecret != "pi_1_secret" || sess.PublishableKey != "pk_test_1" {
		t.Fatalf("create: %v %+v", err, sess)
	}
	if form.Get("amount") != "10260" || form.Get("currency") != "tnd" || form.Get("metadata[order_id]") != "42" {
		t.Fatalf("form sent: %v", form)
	}
	res, err := s.Status(context.Background(), cfg, "pi_1")
	if err != nil || res.Status != StatusPaid || res.OrderID != 42 || res.TransactionID != "ch_9" {
		t.Fatalf("status: %v %+v", err, res)
	}
	body := []byte(`{"type":"payment_intent.succeeded","data":{"object":{"id":"pi_1","object":"payment_intent","metadata":{"order_id":"42","source":"cwmerch"}}}}`)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("Stripe-Signature", signTimestamped("whsec_1", time.Now().Unix(), body))
	ev, err := s.ParseWebhook(context.Background(), cfg, req, body)
	if err != nil || ev.Reference != "pi_1" || ev.OrderID != 42 {
		t.Fatalf("webhook: %v %+v", err, ev)
	}
	req.Header.Set("Stripe-Signature", "t=1,v1=00")
	if _, err := s.ParseWebhook(context.Background(), cfg, req, body); err == nil {
		t.Fatal("unsigned webhook accepted")
	}
}

func TestKonnectCreateStatus(t *testing.T) {
	var sent map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "key" {
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/payments/init-payment":
			_ = json.NewDecoder(r.Body).Decode(&sent)
			_, _ = w.Write([]byte(`{"payUrl":"https://pay/1","paymentRef":"ref1"}`))
		case "/payments/ref1":
			_, _ = w.Write([]byte(`{"payment":{"id":"ref1","status":"completed","amount":25500,"reachedAmount":25500,"token":"TND","orderId":"order-7","transactions":[{"id":"tx1","status":"success"}]}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	k := Konnect{BaseURL: srv.URL}
	cfg := Config{Settings: map[string]any{"walletId": "w1"}, Secrets: map[string]string{"apiKey": "key"}}
	sess, err := k.CreatePayment(context.Background(), cfg, Request{OrderID: 7, Amount: 25.5, Currency: "TND", WebhookURL: "https://api/wh"})
	if err != nil || sess.RedirectURL != "https://pay/1" || sess.Reference != "ref1" {
		t.Fatalf("create: %v %+v", err, sess)
	}
	if sent["amount"].(float64) != 25500 || sent["receiverWalletId"] != "w1" || sent["orderId"] != "order-7" {
		t.Fatalf("sent: %v", sent)
	}
	res, err := k.Status(context.Background(), cfg, "ref1")
	if err != nil || res.Status != StatusPaid || res.OrderID != 7 || res.TransactionID != "tx1" {
		t.Fatalf("status: %v %+v", err, res)
	}
	if err := k.Test(context.Background(), cfg); err != nil {
		t.Fatalf("test with a valid key: %v", err)
	}
}

func TestFlouciCreateStatusWebhook(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer pub:priv" {
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/generate_payment":
			_, _ = w.Write([]byte(`{"result":{"success":true,"payment_id":"p1","link":"https://checkout/p1"}}`))
		case "/verify_payment/p1":
			_, _ = w.Write([]byte(`{"success":true,"result":{"status":"SUCCESS","amount":12000,"developer_tracking_id":"order-9","details":{"order_number":"FL-1"}}}`))
		}
	}))
	defer srv.Close()
	f := Flouci{BaseURL: srv.URL}
	cfg := Config{Settings: map[string]any{"publicKey": "pub"}, Secrets: map[string]string{"privateKey": "priv"}}
	if _, err := f.CreatePayment(context.Background(), cfg, Request{OrderID: 9, Amount: 12, Currency: "EUR"}); err == nil {
		t.Fatal("non-TND accepted")
	}
	sess, err := f.CreatePayment(context.Background(), cfg, Request{OrderID: 9, Amount: 12, Currency: "TND"})
	if err != nil || sess.RedirectURL != "https://checkout/p1" {
		t.Fatalf("create: %v %+v", err, sess)
	}
	res, err := f.Status(context.Background(), cfg, "p1")
	if err != nil || res.Status != StatusPaid || res.OrderID != 9 || res.TransactionID != "FL-1" {
		t.Fatalf("status: %v %+v", err, res)
	}
	body := []byte(`{"event":"payment.succeeded","payment_id":"p1","developer_tracking_id":"order-9"}`)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	req.Header.Set("X-Flouci-Signature", signTimestamped("priv", time.Now().Unix(), body))
	ev, err := f.ParseWebhook(context.Background(), cfg, req, body)
	if err != nil || ev.Reference != "p1" || ev.OrderID != 9 {
		t.Fatalf("webhook: %v %+v", err, ev)
	}
}

func TestEnvName(t *testing.T) {
	if envName("stripe", "secretKey") != "STRIPE_SECRET_KEY" || envName("konnect", "walletId") != "KONNECT_WALLET_ID" {
		t.Fatal(envName("stripe", "secretKey"))
	}
}
