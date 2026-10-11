package layout

import "testing"

// Same-basename directories share a snapshot ID but never a repository.
func TestServiceRepoPerDirectory(t *testing.T) {
	l := Default()
	a, b := l.ServiceRepo("h-app", "/srv/app"), l.ServiceRepo("h-app", "/other/app")
	if a == b {
		t.Fatalf("both directories use %s", a)
	}
	if again := l.ServiceRepo("h-app", "/srv/app"); again != a {
		t.Fatalf("not stable: %s then %s", a, again)
	}
}
