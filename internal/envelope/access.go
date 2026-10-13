package envelope

import (
	"fmt"

	"github.com/SisyphusMD/archiver/internal/config"
)

// Access is exactly the credential selection printed for a storage. Rotating tokens
// cannot be printed; their sign-in step remains a person's responsibility.
func (s Source) Access(n int, typ string) (config.Values, bool) {
	v, _, rotates := s.access(n, typ)
	return v, rotates
}

func (s Source) access(n int, typ string) (config.Values, bool, bool) {
	pre := fmt.Sprintf("STORAGE_TARGET_%d_", n)
	t := config.Types[typ]
	v := valuesOf(s, pre, t)
	bg := false
	for _, f := range t.Fields {
		if f.BreakGlass && s.Settings.Secret(pre+"BREAKGLASS_"+f.Name) != "" {
			bg = true
		}
	}
	for _, f := range t.Fields {
		if f.BreakGlass && s.Settings.Secret(pre+"BREAKGLASS_"+f.Name) == "" {
			bg = false
		}
	}
	rotates := false
	for _, f := range t.Fields {
		if f.Rotates {
			rotates = true
			continue
		}
		if f.Secret {
			v[f.Name] = s.Settings.Secret(pre + f.Name)
			if bg && f.BreakGlass {
				v[f.Name] = s.Settings.Secret(pre + "BREAKGLASS_" + f.Name)
			}
		}
	}
	return v, bg, rotates
}

// SSHAccess uses a break-glass user only when its key is also printed.
func (s Source) SSHAccess(n int) (key, user string, breakglass bool) {
	pre := fmt.Sprintf("STORAGE_TARGET_%d_", n)
	key = s.Settings.Secret(pre + "BREAKGLASS_SSH_KEY")
	breakglass = key != ""
	if !breakglass {
		key = s.sshKey()
	}
	user = s.get(pre + "SFTP_USER")
	if bgUser := s.get(pre + "BREAKGLASS_SFTP_USER"); breakglass && bgUser != "" {
		user = bgUser
	}
	return
}
