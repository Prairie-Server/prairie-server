package livetv

import (
	"errors"
	"testing"
)

func TestValidateMediaFetchURL(t *testing.T) {
	t.Parallel()
	if err := ValidateMediaFetchURL("http://192.168.1.50/discover.json"); err != nil {
		t.Fatalf("LAN http should be allowed: %v", err)
	}
	if err := ValidateMediaFetchURL("https://guide.example/xmltv.xml"); err != nil {
		t.Fatalf("public https should be allowed: %v", err)
	}
	cases := []string{
		"file:///etc/passwd",
		"http://169.254.169.254/latest/meta-data/",
		"http://user:pass@192.168.1.1/lineup.json",
		"http://metadata.google.internal/",
		"http://127.0.0.1/stream",
		"http://localhost/stream",
		"http://[::1]/stream",
		"http://[::ffff:127.0.0.1]/stream",
		"not a url",
	}
	for _, raw := range cases {
		err := ValidateMediaFetchURL(raw)
		if err == nil || !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("ValidateMediaFetchURL(%q) = %v, want ErrInvalidArgument", raw, err)
		}
	}
}

func allowLoopbackMediaFetch(t *testing.T) {
	t.Helper()
	testingAllowLoopback = true
	t.Cleanup(func() { testingAllowLoopback = false })
}
