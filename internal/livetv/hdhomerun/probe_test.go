package hdhomerun

import (
	"testing"
)

func TestProbeCandidateURLsDispatcharr(t *testing.T) {
	got := ProbeCandidateURLs("http://dispatcharr.local:8080")
	want := []string{
		"http://dispatcharr.local:8080/hdhr/discover.json",
		"http://dispatcharr.local:8080/discover.json",
		"http://dispatcharr.local:9191/hdhr/discover.json",
		"https://dispatcharr.local:9191/hdhr/discover.json",
	}
	if len(got) < len(want) {
		t.Fatalf("got %v", got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("got[%d]=%q want %q (full=%v)", i, got[i], w, got)
		}
	}
}

func TestProbeCandidateURLsHDHRPath(t *testing.T) {
	got := ProbeCandidateURLs("http://192.168.1.9:9191/hdhr")
	if got[0] != "http://192.168.1.9:9191/hdhr/discover.json" {
		t.Fatalf("got %v", got)
	}
}

func TestClassifyKind(t *testing.T) {
	if kind := ClassifyKind(&DeviceInfo{
		FriendlyName: "Dispatcharr",
		BaseURL:      "http://x:9191/hdhr",
	}, "http://x:9191/hdhr/discover.json"); kind != "dispatcharr" {
		t.Fatalf("kind=%q", kind)
	}
	if kind := ClassifyKind(&DeviceInfo{
		ModelNumber:  "HDHR5-4K",
		FriendlyName: "HDHomeRun FLEX",
	}, "http://192.168.1.50/discover.json"); kind != "hdhomerun" {
		t.Fatalf("kind=%q", kind)
	}
}
