package configuration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	snapshotSchema      = "aegisadmin.configuration.snapshot.v1"
	maximumSnapshotSize = 32 * 1024 * 1024
)

var defaultSnapshotDir = "/var/lib/aegisadmin-system/configuration/snapshots"

var snapshotIDPattern = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}Z-[a-f0-9]{12}$`)

type Snapshot struct {
	Schema          string         `json:"schema"`
	ID              string         `json:"id"`
	CreatedAt       string         `json:"created_at"`
	InventorySchema string         `json:"inventory_schema"`
	InventorySHA256 string         `json:"inventory_sha256"`
	Inventory       map[string]any `json:"inventory"`
}

type snapshotDocument struct {
	Schema          string          `json:"schema"`
	ID              string          `json:"id"`
	CreatedAt       string          `json:"created_at"`
	InventorySchema string          `json:"inventory_schema"`
	InventorySHA256 string          `json:"inventory_sha256"`
	Inventory       json.RawMessage `json:"inventory"`
}

type snapshotStore struct {
	directory string
	now       func() time.Time
}

func newSnapshotStore(directory string) *snapshotStore {
	return &snapshotStore{directory: directory, now: time.Now}
}

func (s *snapshotStore) create(inventory map[string]any) (Snapshot, error) {
	if err := s.prepare(); err != nil {
		return Snapshot{}, err
	}
	inventoryJSON, err := json.Marshal(inventory)
	if err != nil {
		return Snapshot{}, fmt.Errorf("encode inventory: %w", err)
	}
	digest := sha256.Sum256(inventoryJSON)
	fingerprint := hex.EncodeToString(digest[:])
	createdAt := s.now().UTC().Truncate(time.Second)
	snapshot := Snapshot{
		Schema:          snapshotSchema,
		ID:              createdAt.Format("20060102T150405Z") + "-" + fingerprint[:12],
		CreatedAt:       createdAt.Format(time.RFC3339),
		InventorySchema: stringValue(inventory["schema"]),
		InventorySHA256: fingerprint,
		Inventory:       inventory,
	}
	content, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return Snapshot{}, fmt.Errorf("encode snapshot: %w", err)
	}
	content = append(content, '\n')
	if len(content) > maximumSnapshotSize {
		return Snapshot{}, errors.New("snapshot exceeds maximum size")
	}
	target := filepath.Join(s.directory, snapshot.ID+".json")
	if err = writeImmutable(target, content); err != nil {
		if errors.Is(err, os.ErrExist) {
			existing, readErr := s.get(snapshot.ID)
			if readErr == nil {
				return existing, nil
			}
		}
		return Snapshot{}, err
	}
	if err = s.writeMetadata(snapshot.ID, snapshotMetadata{Name: "Snapshot " + snapshot.CreatedAt, Source: inventorySource(inventory)}); err != nil {
		_ = os.Chmod(target, 0640)
		_ = os.Remove(target)
		return Snapshot{}, err
	}
	return snapshot, nil
}

func (s *snapshotStore) list() ([]map[string]any, error) {
	if err := s.prepare(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.directory)
	if err != nil {
		return nil, err
	}
	items := []map[string]any{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(name, ".json") {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		if !snapshotIDPattern.MatchString(id) {
			continue
		}
		snapshot, readErr := s.get(id)
		if readErr != nil {
			items = append(items, map[string]any{"id": id, "valid": false, "message": "Le snapshot est illisible ou altéré."})
			continue
		}
		available, total := inventoryCounts(snapshot.Inventory)
		metadata := s.metadata(id)
		info, _ := entry.Info()
		items = append(items, map[string]any{
			"id": snapshot.ID, "created_at": snapshot.CreatedAt, "valid": true,
			"name": metadata.Name, "imported": metadata.Imported, "source": metadata.Source,
			"inventory_schema": snapshot.InventorySchema, "inventory_sha256": snapshot.InventorySHA256,
			"available_sections": available, "total_sections": total, "size": func() int64 {
				if info != nil {
					return info.Size()
				}
				return 0
			}(),
		})
	}
	sort.Slice(items, func(i, j int) bool { return stringValue(items[i]["id"]) > stringValue(items[j]["id"]) })
	return items, nil
}

func (s *snapshotStore) get(id string) (Snapshot, error) {
	if !snapshotIDPattern.MatchString(id) {
		return Snapshot{}, errors.New("invalid snapshot identifier")
	}
	if err := s.prepare(); err != nil {
		return Snapshot{}, err
	}
	path := filepath.Join(s.directory, id+".json")
	info, err := os.Lstat(path)
	if err != nil {
		return Snapshot{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > maximumSnapshotSize {
		return Snapshot{}, errors.New("invalid snapshot file")
	}
	file, err := os.Open(path)
	if err != nil {
		return Snapshot{}, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maximumSnapshotSize+1))
	if err != nil || len(content) > maximumSnapshotSize {
		return Snapshot{}, errors.New("snapshot read failed")
	}
	return decodeSnapshot(content, id)
}

func (s *snapshotStore) export(id string) ([]byte, error) {
	if _, err := s.get(id); err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(s.directory, id+".json"))
}

func decodeSnapshot(content []byte, id string) (Snapshot, error) {
	var document snapshotDocument
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return Snapshot{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Snapshot{}, errors.New("invalid trailing snapshot data")
	}
	if document.Schema != snapshotSchema || document.ID != id || document.InventorySchema == "" || len(document.InventorySHA256) != 64 || len(document.Inventory) == 0 {
		return Snapshot{}, errors.New("invalid snapshot structure")
	}
	createdAt, err := time.Parse(time.RFC3339, document.CreatedAt)
	if err != nil || !strings.HasPrefix(id, createdAt.UTC().Format("20060102T150405Z")+"-") {
		return Snapshot{}, errors.New("invalid snapshot metadata")
	}
	// L'empreinte porte sur le JSON compact écrit à la création. La vérifier
	// directement sur les jetons enregistrés évite toute conversion de type
	// (grands entiers, types JSON personnalisés) susceptible de modifier une
	// représentation pourtant intacte.
	var inventoryJSON bytes.Buffer
	if err = json.Compact(&inventoryJSON, document.Inventory); err != nil {
		return Snapshot{}, err
	}
	digest := sha256.Sum256(inventoryJSON.Bytes())
	if !strings.EqualFold(document.InventorySHA256, hex.EncodeToString(digest[:])) || !strings.HasSuffix(id, "-"+document.InventorySHA256[:12]) {
		return Snapshot{}, errors.New("snapshot fingerprint mismatch")
	}
	var inventory map[string]any
	inventoryDecoder := json.NewDecoder(bytes.NewReader(document.Inventory))
	inventoryDecoder.UseNumber()
	if err = inventoryDecoder.Decode(&inventory); err != nil || document.InventorySchema != stringValue(inventory["schema"]) {
		return Snapshot{}, errors.New("invalid snapshot inventory")
	}
	return Snapshot{
		Schema: document.Schema, ID: document.ID, CreatedAt: document.CreatedAt,
		InventorySchema: document.InventorySchema, InventorySHA256: document.InventorySHA256,
		Inventory: inventory,
	}, nil
}

func (s *snapshotStore) importSnapshot(content []byte, name string) (Snapshot, error) {
	if err := s.prepare(); err != nil {
		return Snapshot{}, err
	}
	if len(content) == 0 || len(content) > maximumSnapshotSize {
		return Snapshot{}, errors.New("invalid imported snapshot size")
	}
	var identity struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(content, &identity) != nil || !snapshotIDPattern.MatchString(identity.ID) {
		return Snapshot{}, errors.New("invalid imported snapshot identifier")
	}
	snapshot, err := decodeSnapshot(content, identity.ID)
	if err != nil {
		return Snapshot{}, err
	}
	name, err = normalizeSnapshotName(name)
	if err != nil {
		return Snapshot{}, err
	}
	target := filepath.Join(s.directory, snapshot.ID+".json")
	if _, err = os.Lstat(target); err == nil {
		return Snapshot{}, os.ErrExist
	} else if !errors.Is(err, os.ErrNotExist) {
		return Snapshot{}, err
	}
	if err = writeImmutable(target, content); err != nil {
		return Snapshot{}, err
	}
	metadata := snapshotMetadata{Name: name, Imported: true, ImportedAt: time.Now().UTC().Truncate(time.Second).Format(time.RFC3339), Source: inventorySource(snapshot.Inventory)}
	if err = s.writeMetadata(snapshot.ID, metadata); err != nil {
		_ = os.Chmod(target, 0640)
		_ = os.Remove(target)
		return Snapshot{}, err
	}
	return snapshot, nil
}

func (s *snapshotStore) delete(id string) error {
	if _, err := s.get(id); err != nil {
		return err
	}
	path := filepath.Join(s.directory, id+".json")
	if err := os.Chmod(path, 0640); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		_ = os.Chmod(path, 0440)
		return err
	}
	_ = os.Remove(filepath.Join(s.directory, id+".meta.json"))
	return nil
}

func inventorySource(inventory map[string]any) string {
	for _, section := range indexedSections(inventory) {
		if stringValue(section["id"]) != "system" {
			continue
		}
		data, _ := section["data"].(map[string]any)
		info, _ := data["info"].(map[string]any)
		for _, key := range []string{"hostname", "host", "name"} {
			if value := stringValue(info[key]); value != "" {
				return value
			}
		}
	}
	return ""
}

func (s *snapshotStore) prepare() error {
	if !filepath.IsAbs(s.directory) || filepath.Clean(s.directory) != s.directory {
		return errors.New("invalid snapshot directory")
	}
	if err := os.MkdirAll(s.directory, 0750); err != nil {
		return err
	}
	info, err := os.Lstat(s.directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("invalid snapshot directory")
	}
	return os.Chmod(s.directory, 0750)
}

func writeImmutable(target string, content []byte) error {
	directory := filepath.Dir(target)
	temporary, err := os.CreateTemp(directory, ".snapshot-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err = temporary.Chmod(0440); err == nil {
		_, err = temporary.Write(content)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Link(temporaryPath, target)
}

func inventoryCounts(inventory map[string]any) (int, int) {
	sections, ok := inventory["sections"].([]any)
	if !ok {
		if typed, typedOK := inventory["sections"].([]map[string]any); typedOK {
			available := 0
			for _, section := range typed {
				if value, _ := section["available"].(bool); value {
					available++
				}
			}
			return available, len(typed)
		}
		return 0, 0
	}
	available := 0
	for _, raw := range sections {
		if section, ok := raw.(map[string]any); ok {
			if value, _ := section["available"].(bool); value {
				available++
			}
		}
	}
	return available, len(sections)
}

func stringValue(value any) string { text, _ := value.(string); return text }
