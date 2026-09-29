package harness

import (
	"path/filepath"
	"testing"
)

// StoragePassword encrypts every test storage. Fixed, because it is part of what an
// upgraded installation must keep.
const StoragePassword = "e2e-storage-password"

// Keys is an RSA keypair as archiver deployments hold it: PKCS#1 PEM, private half
// encrypted with the passphrase.
type Keys struct {
	Passphrase  string
	PrivatePath string
	PublicPath  string
}

// NewKeys generates a keypair with the openssl inside image, the way `archiver init`
// does, so the keys have exactly the format deployed installations have.
func NewKeys(t testing.TB, image, dir string) Keys {
	t.Helper()
	k := Keys{
		Passphrase:  "e2e-rsa-passphrase",
		PrivatePath: filepath.Join(dir, "private.pem"),
		PublicPath:  filepath.Join(dir, "public.pem"),
	}
	run := func(args ...string) {
		t.Helper()
		full := append([]string{"run", "--rm", "--entrypoint", "openssl", "-v", dir + ":" + dir, image}, args...)
		if r := docker(t, full...); r.Code != 0 {
			t.Fatalf("openssl %v: %s", args, r.Output())
		}
	}
	run("genrsa", "-aes256", "-passout", "pass:"+k.Passphrase, "-out", k.PrivatePath, "-traditional", "2048")
	run("rsa", "-in", k.PrivatePath, "-passin", "pass:"+k.Passphrase, "-pubout", "-out", k.PublicPath)
	return k
}
