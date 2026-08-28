package hdhomerun

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Devices report TranscodeCodecs as an array on some firmware and a
// comma-separated string on others; every model that cannot transcode omits it.
func TestDiscoverParsesTranscodeCodecs(t *testing.T) {
	cases := map[string]struct {
		body string
		want []string
	}{
		"array": {
			body: `{"DeviceID":"1","BaseURL":"http://tuner","TranscodeCodecs":["heavy","mobile"]}`,
			want: []string{"heavy", "mobile"},
		},
		"comma separated": {
			body: `{"DeviceID":"1","BaseURL":"http://tuner","TranscodeCodecs":"heavy, mobile ,internet480"}`,
			want: []string{"heavy", "mobile", "internet480"},
		},
		"absent (every current model)": {
			body: `{"DeviceID":"1","BaseURL":"http://tuner"}`,
			want: nil,
		},
		"null": {
			body: `{"DeviceID":"1","BaseURL":"http://tuner","TranscodeCodecs":null}`,
			want: nil,
		},
		"empty string": {
			body: `{"DeviceID":"1","BaseURL":"http://tuner","TranscodeCodecs":""}`,
			want: nil,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			info, err := NewClient(srv.Client()).Discover(context.Background(), srv.URL+"/discover.json", "")
			if err != nil {
				t.Fatalf("Discover: %v", err)
			}
			if len(info.TranscodeCodecs) != len(tc.want) {
				t.Fatalf("TranscodeCodecs = %v, want %v", info.TranscodeCodecs, tc.want)
			}
			for i, want := range tc.want {
				if info.TranscodeCodecs[i] != want {
					t.Fatalf("TranscodeCodecs = %v, want %v", info.TranscodeCodecs, tc.want)
				}
			}
		})
	}
}

func TestTranscodeCodecsRejectsUnexpectedShapes(t *testing.T) {
	var codecs transcodeCodecs
	if err := json.Unmarshal([]byte(`{"heavy":true}`), &codecs); err == nil {
		t.Fatal("expected an error for an object")
	}
	if err := json.Unmarshal([]byte(`[1,2]`), &codecs); err == nil {
		t.Fatal("expected an error for a numeric array")
	}
}
