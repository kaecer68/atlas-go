package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/live"
)

func main() {
	var help bool
	flag.BoolVar(&help, "help", false, "show help")
	flag.Parse()
	if help {
		fmt.Println("Usage: validate-broker [--help]")
		fmt.Println("Validates broker adapter signature formatting against a local test server.")
		fmt.Println("Set ATLAS_BROKER_API_KEY, ATLAS_BROKER_API_SECRET and ATLAS_BROKER_KEY_ID for real validation;")
		fmt.Println("otherwise dummy credentials are used for format checks only.")
		os.Exit(0)
	}

	apiKey := os.Getenv("ATLAS_BROKER_API_KEY")
	apiSecret := os.Getenv("ATLAS_BROKER_API_SECRET")
	keyID := os.Getenv("ATLAS_BROKER_KEY_ID")

	if apiKey == "" || apiSecret == "" {
		fmt.Println("Usage: ATLAS_BROKER_API_KEY=... ATLAS_BROKER_API_SECRET=... ATLAS_BROKER_KEY_ID=... go run ./cmd/experimental/validate-broker")
		fmt.Println("If credentials are empty, dummy values are used for signature-format validation only.")
	}

	dummyMode := apiKey == "" || apiSecret == ""
	if dummyMode {
		apiKey = "dummy-key"
		apiSecret = "dummy-secret"
		keyID = "dummy-kid"
	}

	// Start a validation server that checks HMAC-SHA256 signature structure.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/orders" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		if r.Header.Get("X-Api-Key") != apiKey {
			http.Error(w, "invalid api key", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("X-Signature-Method") != "hmac-sha256" {
			http.Error(w, fmt.Sprintf("unexpected signature method: %s", r.Header.Get("X-Signature-Method")), http.StatusBadRequest)
			return
		}
		if r.Header.Get("X-Signature-Version") != "v1" {
			http.Error(w, "unexpected signature version", http.StatusBadRequest)
			return
		}
		if r.Header.Get("X-Key-Id") != keyID {
			http.Error(w, "missing key id", http.StatusBadRequest)
			return
		}

		ts := r.Header.Get("X-Request-Timestamp")
		nonce := r.Header.Get("X-Request-Nonce")
		sig := r.Header.Get("X-Signature")

		payload, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}

		// Validate HMAC using the same canonical input format as the adapter.
		expected := expectedHMAC(payload, apiSecret, ts, nonce)
		if !hmac.Equal([]byte(sig), []byte(expected)) {
			http.Error(w, fmt.Sprintf("invalid signature: got=%s expected=%s", sig, expected), http.StatusUnauthorized)
			return
		}

		var order domain.Order
		if err := json.Unmarshal(payload, &order); err != nil {
			http.Error(w, fmt.Sprintf("invalid payload: %v", err), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"order_id":   "validation-oid",
			"status":     "filled",
			"fill_price": order.Price,
		})
	}))
	defer server.Close()

	adapter := live.NewHTTPBrokerAdapter(live.HTTPBrokerAdapterConfig{
		BaseURL:              server.URL,
		APIKey:               apiKey,
		APISecret:            apiSecret,
		KeyID:                keyID,
		Timeout:              5 * time.Second,
		MaxAttempts:          1,
		Client:               server.Client(),
		Signer:               "hmac-sha256",
		RetryableStatusCodes: []int{408, 425, 429, 500, 502, 503, 504},
	})

	order := domain.Order{
		Symbol:   "2330",
		Side:     domain.SideBuy,
		Quantity: 1,
		Price:    500.0,
		Reason:   "broker validation",
	}

	result, err := adapter.SubmitOrder(context.Background(), order)
	if err != nil {
		log.Fatalf("Broker validation FAILED: %v", err)
	}

	fmt.Printf("Broker validation PASSED\n")
	if dummyMode {
		fmt.Printf("  Mode:        dummy credentials (signature format only)\n")
	} else {
		fmt.Printf("  Mode:        real credentials\n")
	}
	fmt.Printf("  Order ID:    %s\n", result.OrderID)
	fmt.Printf("  Status:      %s\n", result.Status)
	fmt.Printf("  Fill Price:  %.2f\n", result.FillPrice)
	fmt.Printf("  Signer:      hmac-sha256\n")
	fmt.Printf("  Key ID:      %s\n", keyID)
}

// expectedHMAC mirrors the canonical sign input used by HTTPBrokerAdapter.
func expectedHMAC(payload []byte, secret, ts, nonce string) string {
	h := hmac.New(sha256.New, []byte(secret))
	canonical := []byte(secret + "\n" + ts + "\n" + nonce + "\n" + string(payload))
	_, _ = h.Write(canonical)
	return hex.EncodeToString(h.Sum(nil))
}
