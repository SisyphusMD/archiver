package damage

import "testing"

// Output of duplicacy 3.2.5's check -a -persist over a missing chunk shared by two
// revisions, and over an altered metadata chunk.
func TestRevisions(t *testing.T) {
	out := `All chunks referenced by snapshot id1 at revision 1 exist
Chunk 9fac referenced by snapshot id1 at revision 2 does not exist
Some chunks referenced by snapshot id1 at revision 2 are missing
Chunk 9fac referenced by snapshot id1 at revision 3 does not exist
Some chunks referenced by snapshot id1 at revision 3 are missing
Failed to load chunks for snapshot my-db at revision 7: unexpected end of JSON input
Some chunks referenced by snapshot id1 at revision 2 are missing
Some chunks referenced by some snapshots do not exist in the storage
`
	if got := Describe(out); got != "damaged revisions: id1 revision 2, id1 revision 3, my-db revision 7" {
		t.Fatalf("got %q", got)
	}
	if got := Describe("All chunks referenced by snapshot id1 at revision 1 exist\n"); got != "" {
		t.Fatalf("an intact check described damage: %q", got)
	}
}
