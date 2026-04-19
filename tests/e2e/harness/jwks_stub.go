// SPDX-License-Identifier: GPL-2.0-only
package harness

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// apiKeyEntry records the identity claims to return for a registered
// API key. Mirrors the structure Hive's e2e stub uses.
type apiKeyEntry struct {
	id, username, displayName, userType string
}

// StartJWKSStub starts an in-process Passport stub serving:
//   - GET  /v1/jwks            — public key in JWKS format
//   - POST /v1/verify-api-key  — registry-based check; only pre-registered
//     keys or keys added via MintAPIKey are accepted. Each call increments
//     an internal counter exposed via JWKSStub.APIKeyVerifyCount().
//
// The stub is hand-rolled — it does NOT import or reuse Passport's
// real handler code. If Passport's wire format drifts, this stub
// must drift in lockstep so the harness keeps catching mismatches.
//
// The returned struct exposes:
//   - Addr        — host:port the stub listens on.
//   - Stop()      — shuts the stub down (idempotent).
//   - SignJWT(...) — produces a 1-hour token signed by the JWKS key, audience "flow".
//   - SignExpiredJWT(...) — produces a token whose exp is 1 hour in the past.
//   - MintAPIKey(...) — registers a new API key identity for use in tests.
//   - APIKeyVerifyCount() — total /v1/verify-api-key requests since start.
//     After scheme dispatch, this counter MUST stay zero when only Bearer
//     (JWT) traffic is sent — the dispatch sends Bearer to the JWT path
//     only. Tests that send ApiKey-v1 traffic must see the counter advance.
type JWKSStub struct {
	Addr           string
	srv            *http.Server
	signJWT        func(id, username, displayName, userType string) string
	signExpiredJWT func(id, username, displayName, userType string) string
	apiKeyHits     atomic.Int64
	apiKeys        map[string]apiKeyEntry
	apiKeysMu      sync.Mutex
}

// SignJWT produces a token signed by the JWKS stub's key, audience "flow".
func (s *JWKSStub) SignJWT(id, username, displayName, userType string) string {
	return s.signJWT(id, username, displayName, userType)
}

// SignExpiredJWT mints a JWT whose `exp` claim is one hour in the
// past — used by negative-auth tests to confirm the JWT validator
// honours expiration. Same signature/issuer/audience as SignJWT
// otherwise, so any failure is attributable to the exp claim alone.
func (s *JWKSStub) SignExpiredJWT(id, username, displayName, userType string) string {
	return s.signExpiredJWT(id, username, displayName, userType)
}

// MintAPIKey registers a new API key and returns the key string.
// Tests that need a non-default api-key identity call this; tests
// content with the canned service token use it directly.
func (s *JWKSStub) MintAPIKey(key, id, username, displayName, userType string) {
	s.apiKeysMu.Lock()
	defer s.apiKeysMu.Unlock()
	s.apiKeys[key] = apiKeyEntry{id: id, username: username, displayName: displayName, userType: userType}
}

// APIKeyVerifyCount returns the number of /v1/verify-api-key requests
// served since the stub started.
func (s *JWKSStub) APIKeyVerifyCount() int64 {
	return s.apiKeyHits.Load()
}

// Stop shuts the stub's HTTP server down.
func (s *JWKSStub) Stop() {
	s.srv.Close()
}

// StartJWKSStub starts the Passport stub and returns it ready for use.
func StartJWKSStub() *JWKSStub {
	rawKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(fmt.Sprintf("jwks_stub: generate RSA key: %v", err))
	}

	privJWK, err := jwk.FromRaw(rawKey)
	if err != nil {
		panic(fmt.Sprintf("jwks_stub: create JWK from private key: %v", err))
	}
	_ = privJWK.Set(jwk.KeyIDKey, "test-key-1")
	_ = privJWK.Set(jwk.AlgorithmKey, jwa.RS256)

	privSet := jwk.NewSet()
	_ = privSet.AddKey(privJWK)

	pubSet, err := jwk.PublicSetOf(privSet)
	if err != nil {
		panic(fmt.Sprintf("jwks_stub: derive public key set: %v", err))
	}

	jwksBytes, err := json.Marshal(pubSet)
	if err != nil {
		panic(fmt.Sprintf("jwks_stub: marshal JWKS: %v", err))
	}

	stub := &JWKSStub{apiKeys: make(map[string]apiKeyEntry)}
	stub.apiKeys["harness-service-token"] = apiKeyEntry{
		id:          "00000000-0000-0000-0000-000000000099",
		username:    "flow-e2e-apikey",
		displayName: "Flow E2E API Key",
		userType:    "service",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/jwks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(jwksBytes)
	})
	mux.HandleFunc("POST /v1/verify-api-key", func(w http.ResponseWriter, r *http.Request) {
		stub.apiKeyHits.Add(1)
		var req struct {
			Key string `json:"key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"valid": false, "error": "invalid request"})
			return
		}
		stub.apiKeysMu.Lock()
		ent, ok := stub.apiKeys[req.Key]
		stub.apiKeysMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{"valid": false})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"valid": true,
			"key": map[string]any{
				"userId": ent.id,
				"metadata": map[string]any{
					"username":     ent.username,
					"name":         ent.displayName,
					"display_name": ent.displayName,
					"type":         ent.userType,
				},
			},
		})
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("jwks_stub: listen: %v", err))
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)

	stub.Addr = ln.Addr().String()
	stub.srv = srv
	stub.signJWT = func(id, username, displayName, userType string) string {
		now := time.Now()
		tok, err := jwt.NewBuilder().
			Subject(id).
			Issuer("passport-stub").
			Audience([]string{"flow"}).
			IssuedAt(now).
			Expiration(now.Add(1*time.Hour)).
			Claim("username", username).
			Claim("name", displayName).
			Claim("display_name", displayName).
			Claim("type", userType).
			Build()
		if err != nil {
			panic(fmt.Sprintf("jwks_stub: build JWT: %v", err))
		}
		signedBytes, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, privJWK))
		if err != nil {
			panic(fmt.Sprintf("jwks_stub: sign JWT: %v", err))
		}
		return string(signedBytes)
	}

	stub.signExpiredJWT = func(id, username, displayName, userType string) string {
		past := time.Now().Add(-2 * time.Hour)
		tok, err := jwt.NewBuilder().
			Subject(id).
			Issuer("passport-stub").
			Audience([]string{"flow"}).
			IssuedAt(past).
			Expiration(past.Add(1*time.Hour)). // exp = 1h ago
			Claim("username", username).
			Claim("name", displayName).
			Claim("display_name", displayName).
			Claim("type", userType).
			Build()
		if err != nil {
			panic(fmt.Sprintf("jwks_stub: build expired JWT: %v", err))
		}
		signedBytes, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, privJWK))
		if err != nil {
			panic(fmt.Sprintf("jwks_stub: sign expired JWT: %v", err))
		}
		return string(signedBytes)
	}

	return stub
}
