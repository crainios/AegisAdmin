package configuration

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type snapshotMetadata struct {
	Name       string `json:"name"`
	Imported   bool   `json:"imported"`
	ImportedAt string `json:"imported_at,omitempty"`
	Source     string `json:"source,omitempty"`
}

func normalizeSnapshotName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 100 || strings.ContainsAny(name, "\r\n\x00") {
		return "", errors.New("invalid snapshot name")
	}
	return name, nil
}

func (s *snapshotStore) metadata(id string) snapshotMetadata {
	content, err := os.ReadFile(filepath.Join(s.directory, id+".meta.json"))
	if err != nil {
		return snapshotMetadata{Name: id}
	}
	var metadata snapshotMetadata
	if json.Unmarshal(content, &metadata) != nil {
		return snapshotMetadata{Name: id}
	}
	if name, nameErr := normalizeSnapshotName(metadata.Name); nameErr == nil {
		metadata.Name = name
	} else {
		metadata.Name = id
	}
	return metadata
}

func (s *snapshotStore) writeMetadata(id string, metadata snapshotMetadata) error {
	name, err := normalizeSnapshotName(metadata.Name)
	if err != nil || !snapshotIDPattern.MatchString(id) {
		return errors.New("invalid snapshot metadata")
	}
	metadata.Name = name
	content, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	temporary, err := os.CreateTemp(s.directory, ".metadata-*.tmp")
	if err != nil {
		return err
	}
	path := temporary.Name()
	defer os.Remove(path)
	if err = temporary.Chmod(0640); err == nil {
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
	return os.Rename(path, filepath.Join(s.directory, id+".meta.json"))
}

func (s *snapshotStore) rename(id, name string) error {
	if _, err := s.get(id); err != nil {
		return err
	}
	metadata := s.metadata(id)
	metadata.Name = name
	return s.writeMetadata(id, metadata)
}
