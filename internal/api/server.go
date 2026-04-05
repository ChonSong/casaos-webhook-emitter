package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/chonSong/casaos-webhook-emitter/internal/delivery"
	"github.com/chonSong/casaos-webhook-emitter/internal/registry"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

type Server struct {
	reg    *registry.Registry
	engine *delivery.Engine
	listen string
}

func New(reg *registry.Registry, engine *delivery.Engine, listen string) *Server {
	return &Server{reg: reg, engine: engine, listen: listen}
}

func (s *Server) Start() error {
	r := mux.NewRouter()
	r.HandleFunc("/webhooks", s.handleList).Methods(http.MethodGet)
	r.HandleFunc("/webhooks", s.handleCreate).Methods(http.MethodPost)
	r.HandleFunc("/webhooks/{id}", s.handleDelete).Methods(http.MethodDelete)
	r.HandleFunc("/webhooks/{id}/deliveries", s.handleHistory).Methods(http.MethodGet)
	r.HandleFunc("/webhooks/{id}/test", s.handleTest).Methods(http.MethodPost)
	r.HandleFunc("/health", s.handleHealth).Methods(http.MethodGet)
	r.HandleFunc("/metrics", s.handleMetrics).Methods(http.MethodGet)

	addr := fmt.Sprintf("%s", s.listen)
	log.Printf("[api] management server listening on %s", addr)
	return http.ListenAndServe(addr, r)
}

type webhookInput struct {
	URL    string   `json:"url"`
	Events []string `json:"events"`
	Secret string   `json:"secret,omitempty"`
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]interface{}{
		"webhooks": s.reg.List(),
	})
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var input webhookInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	wh := registry.Webhook{
		ID:     "wh_" + uuid.NewString()[:12],
		URL:    input.URL,
		Events: input.Events,
		Secret: input.Secret,
		Enabled: true,
	}
	if err := s.reg.Add(wh); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(wh)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if !s.reg.Remove(id) {
		http.Error(w, "webhook not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	deliveries := s.engine.GetHistory(id)
	json.NewEncoder(w).Encode(map[string]interface{}{"deliveries": deliveries})
}

func (s *Server) handleTest(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	wh := s.reg.Get(id)
	if wh == nil {
		http.Error(w, "webhook not found", http.StatusNotFound)
		return
	}
	// TODO: fire a test event through the delivery engine
	log.Printf("[api] test webhook %s at %s", wh.ID, wh.URL)
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "test_sent"})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"service": "casaos-webhook-emitter",
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	// Prometheus-style placeholder
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "# casaos_webhook_emitter\nexporter_up 1\n")
}
