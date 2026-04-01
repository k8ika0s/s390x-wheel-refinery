package cas

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	ociManifestMediaType = "application/vnd.oci.image.manifest.v1+json"
	ociConfigMediaType   = "application/vnd.unknown.config.v1+json"
)

type ociDescriptor struct {
	MediaType string            `json:"mediaType"`
	Digest    string            `json:"digest"`
	Size      int64             `json:"size"`
	URLs      []string          `json:"urls,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type ociManifest struct {
	SchemaVersion int            `json:"schemaVersion"`
	MediaType     string         `json:"mediaType"`
	Config        ociDescriptor  `json:"config"`
	Layers        []ociDescriptor `json:"layers"`
	Annotations   map[string]string `json:"annotations,omitempty"`
}

func refForDigest(digest string) string {
	return strings.ReplaceAll(digest, ":", "-")
}

func blobDigest(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func buildManifestPayload(ref string, idDigest string, layerDigest string, layerSize int64, layerMediaType string) ([]byte, string, []byte, error) {
	configPayload := []byte("{}")
	configDigest := blobDigest(configPayload)
	manifest := ociManifest{
		SchemaVersion: 2,
		MediaType:     ociManifestMediaType,
		Config: ociDescriptor{
			MediaType: ociConfigMediaType,
			Digest:    configDigest,
			Size:      int64(len(configPayload)),
			Annotations: map[string]string{
				"org.opencontainers.image.ref.name": ref,
			},
		},
		Layers: []ociDescriptor{{
			MediaType: layerMediaType,
			Digest:    layerDigest,
			Size:      layerSize,
		}},
		Annotations: map[string]string{
			"org.opencontainers.image.title":       ref,
			"io.refinery.artifact.digest":          idDigest,
			"io.refinery.artifact.content_digest":  layerDigest,
			"io.refinery.artifact.layer.mediaType": layerMediaType,
		},
	}
	payload, err := json.Marshal(manifest)
	if err != nil {
		return nil, "", nil, err
	}
	return payload, configDigest, configPayload, nil
}

func parseManifest(payload []byte) (ociManifest, error) {
	var manifest ociManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return ociManifest{}, err
	}
	if manifest.SchemaVersion != 2 {
		return ociManifest{}, fmt.Errorf("unsupported schemaVersion %d", manifest.SchemaVersion)
	}
	if len(manifest.Layers) == 0 {
		return ociManifest{}, fmt.Errorf("manifest missing layers")
	}
	return manifest, nil
}
