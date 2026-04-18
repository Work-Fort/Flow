// SPDX-License-Identifier: GPL-2.0-only
package harness

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

// SharkfinMessage records a message Flow posted to a channel via the
// adapter. Tests assert against Messages() to verify outbound chat
// behaviour.
type SharkfinMessage struct {
	ID       int64
	Channel  string
	Body     string
	Metadata map[string]any
}

// SharkfinChannelCreate records a CreateChannel call.
type SharkfinChannelCreate struct {
	Name   string
	Public bool
}

// FakeSharkfin is a hand-rolled HTTP fake of the Sharkfin REST surface
// Flow uses. It intentionally does NOT import sharkfinclient or reuse
// Sharkfin's real handlers — drift between this fake and Sharkfin's
// wire format must surface in tests.
type FakeSharkfin struct {
	mu             sync.Mutex
	registered     bool
	channels       []SharkfinChannelCreate
	messages       []SharkfinMessage
	webhooks       []string
	nextMessageID  int64
	nextWebhookSeq atomic.Int64
}

func NewFakeSharkfin() *FakeSharkfin { return &FakeSharkfin{} }

// Registered reports whether Flow's startup register call was made.
func (s *FakeSharkfin) Registered() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.registered
}

// Channels returns CreateChannel calls in order.
func (s *FakeSharkfin) Channels() []SharkfinChannelCreate {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SharkfinChannelCreate, len(s.channels))
	copy(out, s.channels)
	return out
}

// Messages returns SendMessage calls in order.
func (s *FakeSharkfin) Messages() []SharkfinMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SharkfinMessage, len(s.messages))
	copy(out, s.messages)
	return out
}

// Webhooks returns the URLs Flow registered.
func (s *FakeSharkfin) Webhooks() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.webhooks))
	copy(out, s.webhooks)
	return out
}

// Start begins serving on a random port. Returns the base URL and a stop fn.
func (s *FakeSharkfin) Start() (baseURL string, stop func()) {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/v1/auth/register", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.registered = true
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/v1/channels", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name   string `json:"name"`
			Public bool   `json:"public"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.channels = append(s.channels, SharkfinChannelCreate{Name: req.Name, Public: req.Public})
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"name": req.Name, "public": req.Public})
	})

	mux.HandleFunc("POST /api/v1/channels/", func(w http.ResponseWriter, r *http.Request) {
		// Path is /api/v1/channels/{name}/{action}.
		rest := strings.TrimPrefix(r.URL.Path, "/api/v1/channels/")
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) != 2 {
			http.NotFound(w, r)
			return
		}
		channel := parts[0]
		action := parts[1]
		switch action {
		case "join":
			w.WriteHeader(http.StatusNoContent)
		case "messages":
			var req struct {
				Body     string         `json:"body"`
				Metadata map[string]any `json:"metadata,omitempty"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.mu.Lock()
			s.nextMessageID++
			id := s.nextMessageID
			s.messages = append(s.messages, SharkfinMessage{
				ID: id, Channel: channel, Body: req.Body, Metadata: req.Metadata,
			})
			s.mu.Unlock()
			writeJSON(w, http.StatusOK, map[string]any{"id": id})
		default:
			http.NotFound(w, r)
		}
	})

	mux.HandleFunc("POST /api/v1/webhooks", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		s.webhooks = append(s.webhooks, req.URL)
		s.mu.Unlock()
		seq := s.nextWebhookSeq.Add(1)
		writeJSON(w, http.StatusOK, map[string]any{"id": "wh-" + itoaSeq(seq)})
	})

	mux.HandleFunc("GET /api/v1/webhooks", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		urls := make([]string, len(s.webhooks))
		copy(urls, s.webhooks)
		s.mu.Unlock()
		out := make([]map[string]any, len(urls))
		for i, u := range urls {
			out[i] = map[string]any{"id": "wh-" + itoaSeq(int64(i+1)), "url": u, "active": true}
		}
		writeJSON(w, http.StatusOK, out)
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic("fake_sharkfin: listen: " + err.Error())
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)

	return "http://" + ln.Addr().String(), func() { srv.Close() }
}

func itoaSeq(n int64) string {
	// avoid strconv import here just to keep dependencies minimal
	return formatInt(n)
}

func formatInt(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
