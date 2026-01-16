package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var pythonRecipeRe = regexp.MustCompile(`^cpython(\d{2,3})\.sh$`)
var pythonVersionRe = regexp.MustCompile(`^3\.[0-9]{1,2}$`)

type pythonRecipeInfo struct {
	Version   string `json:"version"`
	Name      string `json:"name"`
	Filename  string `json:"filename"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type pythonRecipeDetail struct {
	pythonRecipeInfo
	Recipe string `json:"recipe"`
}

func (h *Handler) pythonVersions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	list, err := h.listPythonRecipes()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (h *Handler) pythonVersionByID(w http.ResponseWriter, r *http.Request) {
	version := strings.TrimPrefix(r.URL.Path, "/api/python-versions/")
	version = strings.TrimSuffix(version, "/")
	if version == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing python version"})
		return
	}
	if !pythonVersionRe.MatchString(version) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid python version"})
		return
	}
	info, path, err := h.pythonRecipePath(version)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	switch r.Method {
	case http.MethodGet:
		data, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "recipe not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if stat, err := os.Stat(path); err == nil {
			info.UpdatedAt = stat.ModTime().UTC().Format(time.RFC3339)
		}
		writeJSON(w, http.StatusOK, pythonRecipeDetail{pythonRecipeInfo: info, Recipe: string(data)})
	case http.MethodPut:
		if err := h.requireUIToken(r); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
			return
		}
		recipe, err := readRecipeBody(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if strings.TrimSpace(recipe) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "recipe is empty"})
			return
		}
		if err := os.MkdirAll(h.Config.PythonRecipesDir, 0o755); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if err := os.WriteFile(path, []byte(recipe), 0o755); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if stat, err := os.Stat(path); err == nil {
			info.UpdatedAt = stat.ModTime().UTC().Format(time.RFC3339)
		}
		writeJSON(w, http.StatusOK, pythonRecipeDetail{pythonRecipeInfo: info, Recipe: recipe})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (h *Handler) listPythonRecipes() ([]pythonRecipeInfo, error) {
	dir := h.Config.PythonRecipesDir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	list := make([]pythonRecipeInfo, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := pythonRecipeRe.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		version, err := tagToVersion(match[1])
		if err != nil {
			continue
		}
		info := pythonRecipeInfo{
			Version:  version,
			Name:     "cpython" + match[1],
			Filename: entry.Name(),
		}
		if stat, err := entry.Info(); err == nil {
			info.UpdatedAt = stat.ModTime().UTC().Format(time.RFC3339)
		}
		list = append(list, info)
	}
	sort.Slice(list, func(i, j int) bool {
		return versionLess(list[i].Version, list[j].Version)
	})
	return list, nil
}

func (h *Handler) pythonRecipePath(version string) (pythonRecipeInfo, string, error) {
	tag, err := versionToTag(version)
	if err != nil {
		return pythonRecipeInfo{}, "", err
	}
	filename := fmt.Sprintf("cpython%s.sh", tag)
	info := pythonRecipeInfo{
		Version:  version,
		Name:     "cpython" + tag,
		Filename: filename,
	}
	return info, filepath.Join(h.Config.PythonRecipesDir, filename), nil
}

func readRecipeBody(r *http.Request) (string, error) {
	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "text/plain") {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	var payload struct {
		Recipe string `json:"recipe"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("invalid json")
	}
	return payload.Recipe, nil
}

func versionToTag(version string) (string, error) {
	if !pythonVersionRe.MatchString(version) {
		return "", fmt.Errorf("invalid python version")
	}
	parts := strings.Split(version, ".")
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid python version")
	}
	return fmt.Sprintf("3%d", minor), nil
}

func tagToVersion(tag string) (string, error) {
	if tag == "" {
		return "", fmt.Errorf("invalid python tag")
	}
	if !strings.HasPrefix(tag, "3") {
		return "", fmt.Errorf("invalid python tag")
	}
	minor, err := strconv.Atoi(tag[1:])
	if err != nil {
		return "", fmt.Errorf("invalid python tag")
	}
	return fmt.Sprintf("3.%d", minor), nil
}

func versionLess(a, b string) bool {
	amajor, aminor := parseVersion(a)
	bmajor, bminor := parseVersion(b)
	if amajor != bmajor {
		return amajor < bmajor
	}
	return aminor < bminor
}

func parseVersion(version string) (int, int) {
	parts := strings.Split(version, ".")
	if len(parts) != 2 {
		return 0, 0
	}
	major, _ := strconv.Atoi(parts[0])
	minor, _ := strconv.Atoi(parts[1])
	return major, minor
}

func containsPythonVersion(list []pythonRecipeInfo, version string) bool {
	for _, item := range list {
		if item.Version == version {
			return true
		}
	}
	return false
}
