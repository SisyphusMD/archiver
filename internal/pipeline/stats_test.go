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

func TestUploadSpike(t *testing.T) {
	const gib = int64(1) << 30
	for _, c := range []struct {
		name     string
		recent   []int64
		uploaded int64
		spike    bool
	}{
		{"too little history", []int64{40 * gib, 50 * gib}, 500 * gib, false},
		{"a whole service again", []int64{30 * gib, 50 * gib, 44 * gib, 63 * gib}, 503 * gib, true},
		{"a busy night", []int64{30 * gib, 50 * gib, 44 * gib, 63 * gib}, 120 * gib, false},
		{"a small service's noise", []int64{1 << 20, 2 << 20, 1 << 20}, 5 * gib, false},
		{"a small service that jumped a lot", []int64{1 << 20, 2 << 20, 1 << 20}, 25 * gib, true},
		{"exactly four times", []int64{10 * gib, 10 * gib, 10 * gib}, 40 * gib, true},
		{"even history uses both middles", []int64{10 * gib, 10 * gib, 30 * gib, 30 * gib}, 80 * gib, true},
	} {
		if spike, _ := uploadSpike(c.recent, c.uploaded); spike != c.spike {
			t.Errorf("%s: spike %v, want %v", c.name, spike, c.spike)
		}
	}
	if _, median := uploadSpike([]int64{9, 1, 5}, 0); median != 5 {
		t.Errorf("median %d, want 5", median)
	}
	if _, median := uploadSpike([]int64{9, 1, 5, 3}, 0); median != 4 {
		t.Errorf("even median %d, want 4", median)
	}
	if got := humanBytes(503 * gib); got != "503.0 GiB" {
		t.Errorf("humanBytes: %s", got)
	}
}
