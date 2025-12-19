package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/objectstore"
	"github.com/k8ika0s/s390x-wheel-refinery/go-worker/internal/plan"
	"golang.org/x/sync/errgroup"
)

type pendingInput struct {
	ID           int64           `json:"id"`
	Filename     string          `json:"filename"`
	Status       string          `json:"status"`
	Error        string          `json:"error,omitempty"`
	SourceType   string          `json:"source_type,omitempty"`
	Digest       string          `json:"digest,omitempty"`
	ObjectBucket string          `json:"object_bucket,omitempty"`
	ObjectKey    string          `json:"object_key,omitempty"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
}

func plannerLoop(ctx context.Context, cfg Config, popURL, statusURL, listURL string, planPool *atomic.Int32, pyVersion, platformTag *atomic.Value) {
	interval := time.Duration(cfg.PlanPollIntervalSec) * time.Second
	if interval <= 0 {
		interval = 15 * time.Second
	}
	client := &http.Client{Timeout: 30 * time.Second}
	inputStore := cfg.ObjectStore()
	batch := cfg.PlanPopBatch
	if batch <= 0 {
		batch = 5
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		ids, err := popPlanIDs(ctx, client, popURL, cfg.WorkerToken, batch)
		if err != nil {
			log.Printf("planner: pop error: %v", err)
			time.Sleep(interval)
			continue
		}
		if len(ids) == 0 {
			time.Sleep(interval)
			continue
		}
		pendingMap := fetchPendingMap(ctx, client, listURL, cfg.WorkerToken)
		g, gctx := errgroup.WithContext(ctx)
		pool := cfg.PlanPoolSize
		if planPool != nil && planPool.Load() > 0 {
			pool = int(planPool.Load())
		}
		if pool <= 0 {
			pool = 2
		}
		g.SetLimit(pool)
		for _, id := range ids {
			pi, ok := pendingMap[id]
			if !ok {
				log.Printf("planner: pending input %s not found in list", id)
				continue
			}
			piCopy := pi
			g.Go(func() error {
				localCfg := cfg
				if pyVersion != nil {
					if v, ok := pyVersion.Load().(string); ok && v != "" {
						localCfg.PythonVersion = v
					}
				}
				if platformTag != nil {
					if v, ok := platformTag.Load().(string); ok && v != "" {
						localCfg.PlatformTag = v
					}
				}
				if err := planOne(gctx, client, localCfg, inputStore, piCopy, statusURL); err != nil {
					log.Printf("planner: failed planning id=%d: %v", piCopy.ID, err)
				}
				return nil
			})
		}
		_ = g.Wait()
	}
}

func popPlanIDs(ctx context.Context, client *http.Client, popURL, token string, batch int) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s?max=%d", popURL, batch), nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("X-Worker-Token", token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("pop status %d: %s", resp.StatusCode, string(body))
	}
	var out struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.IDs, nil
}

func fetchPendingMap(ctx context.Context, client *http.Client, listURL, token string) map[string]pendingInput {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return map[string]pendingInput{}
	}
	if token != "" {
		req.Header.Set("X-Worker-Token", token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return map[string]pendingInput{}
	}
	defer resp.Body.Close()
	var list []pendingInput
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return map[string]pendingInput{}
	}
	mp := make(map[string]pendingInput, len(list))
	for _, pi := range list {
		mp[fmt.Sprintf("%d", pi.ID)] = pi
	}
	return mp
}

type pendingMeta struct {
	Type         string         `json:"type"`
	Requirements []plan.DepSpec `json:"requirements,omitempty"`
	Wheel        *pendingWheel  `json:"wheel,omitempty"`
	Requires     []plan.DepSpec `json:"requires,omitempty"`
}

type pendingWheel struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	PythonTag   string `json:"python_tag"`
	AbiTag      string `json:"abi_tag"`
	PlatformTag string `json:"platform_tag"`
}

type wheelMeta struct {
	Name        string
	Version     string
	PythonTag   string
	AbiTag      string
	PlatformTag string
}

func inputSetFromPending(ctx context.Context, cfg Config, pi pendingInput, store objectstore.Store) (plan.InputSet, error) {
	var meta pendingMeta
	if len(pi.Metadata) > 0 {
		if err := json.Unmarshal(pi.Metadata, &meta); err != nil {
			return plan.InputSet{}, fmt.Errorf("parse metadata: %w", err)
		}
	}
	kind := pi.SourceType
	if kind == "" {
		kind = meta.Type
	}
	switch kind {
	case "requirements":
		reqs := meta.Requirements
		if len(reqs) == 0 {
			data, err := fetchInputObject(ctx, cfg, pi, store)
			if err != nil {
				return plan.InputSet{}, fmt.Errorf("no requirements metadata for %s: %w", pi.Filename, err)
			}
			reqs = parseRequirementsBytes(data)
		}
		if len(reqs) == 0 {
			return plan.InputSet{}, fmt.Errorf("no requirements metadata for %s", pi.Filename)
		}
		return plan.InputSet{Requirements: reqs}, nil
	case "wheel":
		metaWheel := meta.Wheel
		reqs := meta.Requires
		if metaWheel == nil || metaWheel.Name == "" || metaWheel.Version == "" {
			data, err := fetchInputObject(ctx, cfg, pi, store)
			if err != nil {
				return plan.InputSet{}, fmt.Errorf("no wheel metadata for %s: %w", pi.Filename, err)
			}
			info, err := parseWheelFilename(pi.Filename)
			if err != nil {
				return plan.InputSet{}, fmt.Errorf("parse wheel filename: %w", err)
			}
			metaWheel = &pendingWheel{
				Name:        info.Name,
				Version:     info.Version,
				PythonTag:   info.PythonTag,
				AbiTag:      info.AbiTag,
				PlatformTag: info.PlatformTag,
			}
			if len(reqs) == 0 {
				reqs, _ = parseRequiresDistBytes(data)
			}
		}
		if metaWheel == nil || metaWheel.Name == "" || metaWheel.Version == "" {
			return plan.InputSet{}, fmt.Errorf("no wheel metadata for %s", pi.Filename)
		}
		w := plan.WheelInput{
			Filename:    pi.Filename,
			Name:        metaWheel.Name,
			Version:     metaWheel.Version,
			PythonTag:   metaWheel.PythonTag,
			AbiTag:      metaWheel.AbiTag,
			PlatformTag: metaWheel.PlatformTag,
			Digest:      pi.Digest,
			Requires:    reqs,
		}
		return plan.InputSet{Wheels: []plan.WheelInput{w}}, nil
	default:
		if strings.HasSuffix(strings.ToLower(pi.Filename), ".whl") {
			return plan.InputSet{Wheels: []plan.WheelInput{{Filename: pi.Filename, Digest: pi.Digest}}}, nil
		}
		if len(meta.Requirements) > 0 {
			return plan.InputSet{Requirements: meta.Requirements}, nil
		}
		return plan.InputSet{}, fmt.Errorf("unsupported input type for %s", pi.Filename)
	}
}

func planOne(ctx context.Context, client *http.Client, cfg Config, store objectstore.Store, pi pendingInput, statusURL string) error {
	inputs, err := inputSetFromPending(ctx, cfg, pi, store)
	if err != nil {
		return err
	}
	hints, err := fetchHints(ctx, client, cfg)
	if err != nil {
		log.Printf("planner: fetch hints failed: %v", err)
	}
	cacheDir := cfg.CacheDir
	if cacheDir != "" && pi.ID > 0 {
		cacheDir = filepath.Join(cacheDir, "plans", fmt.Sprintf("%d", pi.ID))
	}
	snap, err := plan.GenerateFromInputs(
		inputs,
		cacheDir,
		cfg.PythonVersion,
		cfg.PlatformTag,
		cfg.IndexURL,
		cfg.ExtraIndexURL,
		cfg.UpgradeStrategy,
		cfg.ConstraintsPath,
		hints,
		cfg.PackCatalog,
		cfg.CASStore(),
		cfg.CASRegistryURL,
		cfg.CASRegistryRepo,
	)
	statusBody := map[string]string{"status": "planned"}
	if err != nil {
		statusBody["status"] = "failed"
		statusBody["error"] = err.Error()
	}
	if postErr := updatePendingStatus(ctx, client, statusURL, cfg.WorkerToken, pi.ID, statusBody); postErr != nil {
		log.Printf("planner: status update failed for id %d: %v", pi.ID, postErr)
	}
	if err != nil {
		return err
	}
	if err := postPlan(ctx, client, cfg, snap, pi.ID); err != nil {
		return fmt.Errorf("post plan: %w", err)
	}
	return nil
}

func updatePendingStatus(ctx context.Context, client *http.Client, statusURL, token string, id int64, body map[string]string) error {
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/%d", statusURL, id), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Worker-Token", token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func fetchInputObject(ctx context.Context, cfg Config, pi pendingInput, store objectstore.Store) ([]byte, error) {
	if pi.ObjectKey == "" {
		return nil, fmt.Errorf("object key missing")
	}
	if cfg.ObjectStoreBucket != "" && pi.ObjectBucket != "" && cfg.ObjectStoreBucket != pi.ObjectBucket {
		return nil, fmt.Errorf("object bucket mismatch: %s", pi.ObjectBucket)
	}
	if store == nil {
		return nil, fmt.Errorf("object store not configured")
	}
	data, _, err := store.Get(ctx, pi.ObjectKey)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func parseRequirementsBytes(data []byte) []plan.DepSpec {
	lines := strings.Split(string(data), "\n")
	out := make([]plan.DepSpec, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "-c ") || strings.HasPrefix(line, "--constraint") {
			continue
		}
		if idx := strings.Index(line, " #"); idx != -1 {
			line = strings.TrimSpace(line[:idx])
		}
		name := line
		version := ""

		// Handle direct URL or VCS references (`package @ url`)
		if at := strings.Index(line, "@"); at > 0 {
			name = strings.TrimSpace(line[:at])
			version = strings.TrimSpace(line[at:])
		} else {
			// Handle comparison operators, including <= and <
			for _, op := range []string{"==", ">=", "<=", "~=", "<", ">"} {
				if idx := strings.Index(line, op); idx > 0 {
					name = strings.TrimSpace(line[:idx])
					version = strings.TrimSpace(line[idx:])
					// strip leading == for consistency
					if op == "==" {
						version = strings.TrimPrefix(version, "==")
					}
					break
				}
			}
		}

		name = normalizeReqName(name)
		if name == "" {
			continue
		}
		out = append(out, plan.DepSpec{Name: name, Version: version})
	}
	return out
}

func normalizeReqName(name string) string {
	if name == "" {
		return ""
	}
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), "_", "-"))
}

func parseWheelFilename(name string) (wheelMeta, error) {
	base := strings.TrimSuffix(name, ".whl")
	parts := strings.Split(base, "-")
	if len(parts) < 5 {
		return wheelMeta{}, fmt.Errorf("invalid wheel filename: %s", name)
	}
	platform := parts[len(parts)-1]
	abi := parts[len(parts)-2]
	py := parts[len(parts)-3]
	version := parts[len(parts)-4]
	pkgParts := parts[:len(parts)-4]
	pkg := strings.ReplaceAll(strings.Join(pkgParts, "-"), "_", "-")
	return wheelMeta{
		Name:        pkg,
		Version:     version,
		PythonTag:   py,
		AbiTag:      abi,
		PlatformTag: platform,
	}, nil
}

func parseRequiresDistBytes(data []byte) ([]plan.DepSpec, error) {
	reader := bytes.NewReader(data)
	zr, err := zip.NewReader(reader, int64(len(data)))
	if err != nil {
		return nil, err
	}
	var meta []byte
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, "METADATA") {
			rc, err := f.Open()
			if err != nil {
				continue
			}
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(rc)
			rc.Close()
			meta = buf.Bytes()
			break
		}
	}
	if len(meta) == 0 {
		return nil, nil
	}
	return parseRequiresDist(string(meta)), nil
}

func parseRequiresDist(meta string) []plan.DepSpec {
	lines := strings.Split(meta, "\n")
	out := make([]plan.DepSpec, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "requires-dist:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}
			val := strings.TrimSpace(parts[1])
			if semi := strings.Index(val, ";"); semi != -1 {
				val = strings.TrimSpace(val[:semi])
			}
			raw := val
			name := val
			version := ""
			if idx := strings.Index(raw, "("); idx != -1 {
				name = strings.TrimSpace(raw[:idx])
				spec := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(raw[idx:]), "("), ")")
				spec = strings.TrimSpace(spec)
				if strings.HasPrefix(spec, "==") || strings.HasPrefix(spec, ">=") || strings.HasPrefix(spec, "~=") {
					version = spec
					if strings.HasPrefix(version, "==") {
						version = strings.TrimPrefix(version, "==")
					}
				}
			} else {
				for _, op := range []string{"==", ">=", "~="} {
					if idx := strings.Index(raw, op); idx > 0 {
						name = strings.TrimSpace(raw[:idx])
						version = strings.TrimPrefix(strings.TrimSpace(raw[idx:]), "==")
						break
					}
				}
			}
			name = normalizeReqName(name)
			if name == "" {
				continue
			}
			out = append(out, plan.DepSpec{Name: name, Version: version})
		}
	}
	return out
}

func postPlan(ctx context.Context, client *http.Client, cfg Config, snap plan.Snapshot, pendingID int64) error {
	if cfg.ControlPlaneURL == "" {
		return fmt.Errorf("control plane URL not set")
	}
	url := strings.TrimRight(cfg.ControlPlaneURL, "/") + "/api/plan"
	body := map[string]any{"run_id": snap.RunID, "plan": snap.Plan, "dag": snap.DAG}
	if pendingID > 0 {
		body["pending_input_id"] = pendingID
	}
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.ControlPlaneToken != "" {
		req.Header.Set("X-Worker-Token", cfg.ControlPlaneToken)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("plan post status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// Build enqueue is handled by the control-plane when auto-build is enabled.
