package lockstate

import (
	"encoding/json"
	"os"

	"github.com/SisyphusMD/archiver/internal/atomicfile"
)

// KitDrillResult records the envelope recovery path of one storage.
type KitDrillResult struct {
	At             int64   `json:"at"`
	OK             bool    `json:"ok"`
	Seconds        float64 `json:"seconds"`
	LastSuccess    int64   `json:"last_success,omitempty"`
	Message        string  `json:"message,omitempty"`
	SignInRequired bool    `json:"sign_in_required,omitempty"`
}

// ReadKitDrills overlays a run failure even when the per-storage file cannot be written.
func ReadKitDrills(path string) (map[string]KitDrillResult, error) {
	results, err := ReadKitDrillStorages(path)
	run, runErr := ReadKitDrillStorages(path + ".run")
	if runErr != nil {
		return nil, runErr
	}
	if failure, ok := run[""]; ok {
		if results == nil {
			results = map[string]KitDrillResult{}
		}
		for storage, prior := range results {
			prior.At, prior.OK, prior.Message = failure.At, false, failure.Message
			prior.Seconds, prior.SignInRequired = 0, false
			results[storage] = prior
		}
		results[""] = failure
		return results, nil
	}
	return results, err
}

func ReadKitDrillStorages(path string) (map[string]KitDrillResult, error) {
	results := map[string]KitDrillResult{}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return results, nil
	}
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(b, &results)
	if results == nil {
		results = map[string]KitDrillResult{}
	}
	return results, err
}

func WriteKitDrills(path string, results map[string]KitDrillResult) error {
	b, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Write(path, append(b, '\n'), 0o644)
}
