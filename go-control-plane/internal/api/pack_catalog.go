package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/k8ika0s/s390x-wheel-refinery/go-control-plane/internal/packcatalog"
)

// packCatalog handles GET /api/pack-catalog
func (h *Handler) packCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if h.PackCatalog == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pack catalog not loaded"})
		return
	}

	writeJSON(w, http.StatusOK, h.PackCatalog)
}

// packCatalogPacks handles GET /api/pack-catalog/packs
func (h *Handler) packCatalogPacks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if h.PackCatalog == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pack catalog not loaded"})
		return
	}

	packs := h.PackCatalog.ListPacks()
	result := make([]packcatalog.PackDef, 0, len(packs))
	for _, name := range packs {
		if pack, exists := h.PackCatalog.GetPack(name); exists {
			result = append(result, pack)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"packs": result,
		"count": len(result),
	})
}

// packCatalogRuntimes handles GET /api/pack-catalog/runtimes
func (h *Handler) packCatalogRuntimes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if h.PackCatalog == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pack catalog not loaded"})
		return
	}

	runtimes := h.PackCatalog.ListRuntimes()
	result := make([]packcatalog.Runtime, 0, len(runtimes))
	for _, name := range runtimes {
		if runtime, exists := h.PackCatalog.GetRuntime(name); exists {
			result = append(result, runtime)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"runtimes": result,
		"count":    len(result),
	})
}

// packCatalogPackByName handles GET /api/pack-catalog/packs/{name}
func (h *Handler) packCatalogPackByName(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if h.PackCatalog == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pack catalog not loaded"})
		return
	}

	// Extract pack name from path
	path := strings.TrimPrefix(r.URL.Path, "/api/pack-catalog/packs/")
	name := strings.TrimSpace(path)

	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "pack name required"})
		return
	}

	pack, exists := h.PackCatalog.GetPack(name)
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "pack not found"})
		return
	}

	// Get transitive dependencies
	deps := h.PackCatalog.GetPackDependencies(name)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"pack":                    pack,
		"dependencies":            pack.Dependencies,
		"transitive_dependencies": deps,
	})
}

// packCatalogRuntimeByName handles GET /api/pack-catalog/runtimes/{name}
func (h *Handler) packCatalogRuntimeByName(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if h.PackCatalog == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pack catalog not loaded"})
		return
	}

	// Extract runtime name from path
	path := strings.TrimPrefix(r.URL.Path, "/api/pack-catalog/runtimes/")
	name := strings.TrimSpace(path)

	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "runtime name required"})
		return
	}

	runtime, exists := h.PackCatalog.GetRuntime(name)
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "runtime not found"})
		return
	}

	// Get transitive dependencies
	deps := h.PackCatalog.GetRuntimeDependencies(name)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"runtime":                 runtime,
		"dependencies":            runtime.Dependencies,
		"transitive_dependencies": deps,
	})
}

// packCatalogSelect handles POST /api/pack-catalog/select
func (h *Handler) packCatalogSelect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if h.PackCatalog == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pack catalog not loaded"})
		return
	}

	var req struct {
		Package string `json:"package"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Package == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "package name required"})
		return
	}

	selected := h.PackCatalog.SelectPacksForPackage(req.Package)

	// Get full pack definitions
	packs := make([]packcatalog.PackDef, 0, len(selected))
	for _, name := range selected {
		if pack, exists := h.PackCatalog.GetPack(name); exists {
			packs = append(packs, pack)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"package": req.Package,
		"packs":   packs,
		"count":   len(packs),
	})
}

// packCatalogDependencies handles GET /api/pack-catalog/dependencies/{type}/{name}
func (h *Handler) packCatalogDependencies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	if h.PackCatalog == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "pack catalog not loaded"})
		return
	}

	// Extract type and name from path: /api/pack-catalog/dependencies/{type}/{name}
	path := strings.TrimPrefix(r.URL.Path, "/api/pack-catalog/dependencies/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) != 2 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid path format"})
		return
	}

	depType := strings.TrimSpace(parts[0])
	name := strings.TrimSpace(parts[1])

	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name required"})
		return
	}

	var deps []string
	switch depType {
	case "pack", "packs":
		deps = h.PackCatalog.GetPackDependencies(name)
	case "runtime", "runtimes":
		deps = h.PackCatalog.GetRuntimeDependencies(name)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type must be 'pack' or 'runtime'"})
		return
	}

	if deps == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"type":         depType,
		"name":         name,
		"dependencies": deps,
		"count":        len(deps),
	})
}

// Made with Bob
