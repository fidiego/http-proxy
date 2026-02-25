package web

import (
	"encoding/json"
	"net/http"

	"github.com/fidiego/http-proxy/pkg/proxy"
)

type handlers struct {
	engine *proxy.Engine
	hub    *wsHub
}

func (h *handlers) listFlows(w http.ResponseWriter, r *http.Request) {
	flows := h.engine.Store().All()
	jsonOK(w, flows)
}

func (h *handlers) getFlow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	flow := h.engine.Store().Get(id)
	if flow == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	jsonOK(w, flow)
}

func (h *handlers) replayFlow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	flow, err := h.engine.Replay(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	jsonOK(w, flow)
}

func (h *handlers) clearFlows(w http.ResponseWriter, _ *http.Request) {
	h.engine.Store().Clear()
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) getConfig(w http.ResponseWriter, _ *http.Request) {
	upstreams := h.engine.Router().Upstreams()
	type upstreamInfo struct {
		Name   string `json:"name"`
		Prefix string `json:"prefix"`
		Target string `json:"target"`
	}
	infos := make([]upstreamInfo, len(upstreams))
	for i, u := range upstreams {
		infos[i] = upstreamInfo{Name: u.Name, Prefix: u.Prefix, Target: u.Target}
	}
	jsonOK(w, map[string]interface{}{
		"upstreams": infos,
		"flows":     h.engine.Store().Count(),
	})
}

func jsonOK(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
