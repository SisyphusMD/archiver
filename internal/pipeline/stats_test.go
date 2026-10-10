package pipeline

import "testing"

// duplicacy 3.2.5's -stats lines, as a backup prints them.
func TestNoteStats(t *testing.T) {
	b := &Backup{}
	b.noteStats("/srv/app", `Backup for /data/services/app at revision 12 completed
Files: 2 total, 4,101K bytes; 1 new, 3,906K bytes
All chunks: 6 total, 4,102K bytes; 5 new, 3,906K bytes, 3,923K bytes uploaded
`)
	if st := b.stats["/srv/app"]; st.revision != 12 || st.uploaded != 3923*1024 {
		t.Fatalf("%+v", st)
	}
	for in, want := range map[[2]string]int64{{"512", ""}: 512, {"1,234", "K"}: 1234 * 1024, {"2.5", "M"}: 2621440, {"1", "G"}: 1 << 30} {
		if got := parseSize(in[0], in[1]); got != want {
			t.Errorf("%v: %d, want %d", in, got, want)
		}
	}
}
