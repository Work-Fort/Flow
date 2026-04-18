// SPDX-License-Identifier: GPL-2.0-only
package harness

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// StartJWKSStub starts an in-process Passport stub serving:
//   - GET  /v1/jwks            — public key in JWKS format
//   - POST /v1/verify-api-key  — accepts any non-empty key (except the literal
//     "INVALID"), returns a canned identity. Each call increments an internal
//     counter exposed via JWKSStub.APIKeyVerifyCount().
//
// The stub is hand-rolled — it does NOT import or reuse Passport's
// real handler code. If Passport's wire format drifts, this stub
// must drift in lockstep so the harness keeps catching mismatches.
//
// The returned struct exposes:
//   - Addr        — host:port the stub listens on.
//   - Stop()      — shuts the stub down (idempotent).
//   - SignJWT(...) — produces a 1-hour token signed by the JWKS key, audience "flow".
//   - APIKeyVerifyCount() — total /v1/verify-api-key requests since start.
//     Used by future API-key tests to prove the apikey path was actually
//     exercised (the JWT validator runs first in the chain; without this
//     counter, an apikey-validator regression could be masked by the JWT
//     validator returning the same 401).
type JWKSStub struct {
	Addr    string
	srv     *http.Server
	signJWT func(id, username, displayName, userType string) string
	apiKeyHits atomic.Int64
}

// SignJWT produces a token signed by the JWKS stub's key, audience "flow".
func (s *JWKSStub) SignJWT(id, username, displayName, userType string) string {
	return s.signJWT(id, username, displayName, userType)
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

	apiKeyIdentity := map[string]any{
		"valid": true,
		"key": map[string]any{
			"userId": "00000000-0000-0000-0000-000000000099",
			"metadata": map[string]any{
				"username":     "flow-e2e-apikey",
				"name":         "Flow E2E API Key",
				"display_name": "Flow E2E API Key",
				"type":         "service",
			},
		},
	}

	stub := &JWKSStub{}

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
		// Distinguish a "bad" key. The literal "INVALID" is rejected;
		// anything else (including JWT-shaped strings the JWT validator
		// already rejected) returns the canned identity.
		if req.Key == "INVALID" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"valid": false})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(apiKeyIdentity)
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
			Expiration(now.Add(1 * time.Hour)).
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

	return stub
}
