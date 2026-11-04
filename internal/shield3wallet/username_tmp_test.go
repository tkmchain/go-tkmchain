package shield3wallet

import "testing"

func TestUsernameHandleCanonicalVector(t *testing.T) {
	got, err := UsernameHandle("laugh26me", 8979)
	if err != nil {
		t.Fatal(err)
	}
	const want = "@laugh26me#34vouwy"
	if got != want {
		t.Fatalf("UsernameHandle() = %q, want %q", got, want)
	}
	name, err := ParseUsernameHandle(got, 8979)
	if err != nil || name != "laugh26me" {
		t.Fatalf("ParseUsernameHandle() = %q, %v", name, err)
	}
}
