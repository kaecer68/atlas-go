package importer

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/replay"
)

func ImportTWOpenDataCSVToJSONL(sourcePath, targetPath string) error {
	ds, err := replay.LoadTWSEOpenDataCSV(sourcePath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}

	f, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	enc := json.NewEncoder(f)
	for _, date := range ds.Dates {
		for _, bar := range ds.ByDate[date.Format("2006-01-02")] {
			if err := enc.Encode(bar); err != nil {
				return err
			}
		}
	}
	return nil
}
