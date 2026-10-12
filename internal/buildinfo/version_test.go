package buildinfo

import "testing"

func TestStableVersionAndWindowsBounds(t *testing.T) {
	for _, invalid := range []string{"", "v1.0.0", "1.0", "01.0.0", "1.0.0-rc.1", "1.0.0+build", "65536.0.0", "-1.0.0", "1.0.0\n"} {
		if _, err := Parse(invalid); err == nil {
			t.Fatalf("accepted %q", invalid)
		}
	}
	for _, valid := range []string{Current(), "0.9.0", "1.0.0", "65535.65535.65535"} {
		v, err := Parse(valid)
		if err != nil || v.String() != valid {
			t.Fatalf("version %q", valid)
		}
	}
	a, _ := Parse("1.10.0")
	b, _ := Parse("1.2.0")
	if a.Compare(b) != 1 || b.Compare(a) != -1 || a.Compare(a) != 0 {
		t.Fatal("not numeric SemVer comparison")
	}
}
