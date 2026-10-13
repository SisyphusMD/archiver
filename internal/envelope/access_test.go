package envelope

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SisyphusMD/archiver/internal/config"
)

func TestAccessNeverExceedsEnvelope(t *testing.T) {
	for typ, definition := range config.Types {
		if typ == "local" || typ == "sftp" || typ == "sftpc" {
			continue
		}
		t.Run(typ, func(t *testing.T) {
			settings := &config.Settings{}
			var bgFields []string
			for _, f := range definition.Fields {
				name := "STORAGE_TARGET_1_" + f.Name
				if f.Secret {
					settings.Secrets = append(settings.Secrets, config.Setting{Name: name, Value: "backup-" + f.Name})
					if f.BreakGlass {
						bgFields = append(bgFields, f.Name)
						settings.Secrets = append(settings.Secrets, config.Setting{Name: "STORAGE_TARGET_1_BREAKGLASS_" + f.Name, Value: "glass-" + f.Name})
					}
				}
			}
			source := Source{Settings: settings}
			values, rotates := source.Access(1, typ)
			for _, f := range definition.Fields {
				if f.Rotates {
					if !rotates || values[f.Name] != "" {
						t.Fatal("rotating token appeared in envelope access")
					}
					continue
				}
				if f.Secret {
					want := "backup-" + f.Name
					if f.BreakGlass {
						want = "glass-" + f.Name
					}
					if values[f.Name] != want {
						t.Fatalf("wrong credential selection for %s", f.Name)
					}
				}
			}
			if len(bgFields) > 0 {
				name := "STORAGE_TARGET_1_BREAKGLASS_" + bgFields[0]
				for i, kv := range settings.Secrets {
					if kv.Name == name {
						settings.Secrets = append(settings.Secrets[:i], settings.Secrets[i+1:]...)
						break
					}
				}
				values, _ = source.Access(1, typ)
				for _, field := range bgFields {
					if values[field] != "backup-"+field {
						t.Fatalf("partial break-glass set must use the backup set: %s", field)
					}
				}
			}
		})
	}
}

func TestSSHAccessPairsUserWithPrintedKey(t *testing.T) {
	key := filepath.Join(t.TempDir(), "key")
	os.WriteFile(key, []byte("backup-key\n"), 0o600)
	for _, configured := range []bool{false, true} {
		settings := &config.Settings{NonSecret: []config.Setting{{Name: "STORAGE_TARGET_2_SFTP_USER", Value: "backup"}, {Name: "STORAGE_TARGET_2_BREAKGLASS_SFTP_USER", Value: "reader"}}}
		if configured {
			settings.Secrets = []config.Setting{{Name: "STORAGE_TARGET_2_BREAKGLASS_SSH_KEY", Value: "glass-key"}}
		}
		source := Source{Settings: settings, SSHKeyFile: key}
		got, user, bg := source.SSHAccess(2)
		wantKey, wantUser := "backup-key", "backup"
		if configured {
			wantKey, wantUser = "glass-key", "reader"
		}
		if got != wantKey || user != wantUser || bg != configured {
			t.Fatalf("incorrect SSH selection with key configured=%v", configured)
		}
	}
}
