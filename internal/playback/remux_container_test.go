package playback

import (
	"strings"
	"testing"
)

func TestRemuxOutputContainer(t *testing.T) {
	cases := []struct {
		name     string
		source   string
		declared []string
		want     string
	}{
		// The case this exists for: the client can demux the source container, so
		// the remux keeps it instead of rewriting into fragmented MP4. The Tizen
		// client declares exactly this list.
		{"keeps mkv when declared", "mkv", []string{"mp4", "mpegts", "hls", "mkv"}, RemuxContainerMKV},
		// The other reason to remux -- container unsupported -- still gets MP4,
		// which is what such a session needed in the first place.
		{"mp4 when source container not declared", "mkv", []string{"mp4"}, RemuxContainerMP4},
		{"mp4 source stays mp4", "mp4", []string{"mp4", "mkv"}, RemuxContainerMP4},
		// A client that declares nothing gets the historical default.
		{"no declarations falls back", "mkv", nil, RemuxContainerMP4},
		{"empty declarations falls back", "mkv", []string{}, RemuxContainerMP4},
		// Spelling differences between probes and clients must not decide this.
		{"matroska spelling matches mkv", "matroska", []string{"mkv"}, RemuxContainerMKV},
		{"mkv source matches matroska claim", "mkv", []string{"matroska"}, RemuxContainerMKV},
		{"case and space tolerant", " MKV ", []string{"MkV"}, RemuxContainerMKV},
		{"m4v normalizes to mp4", "m4v", []string{"mp4"}, RemuxContainerMP4},
		// Containers we will not write fall back rather than being passed through.
		{"avi source falls back", "avi", []string{"avi"}, RemuxContainerMP4},
		{"unknown source falls back", "", []string{"mkv"}, RemuxContainerMP4},
		// webm is a Matroska subset that cannot carry AC-3 or H.264, so declaring
		// it must never redirect an mkv source into it.
		{"webm claim does not match mkv source", "mkv", []string{"webm"}, RemuxContainerMP4},
		{"webm source falls back", "webm", []string{"webm"}, RemuxContainerMP4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RemuxOutputContainer(tc.source, tc.declared); got != tc.want {
				t.Errorf("RemuxOutputContainer(%q, %v) = %q, want %q", tc.source, tc.declared, got, tc.want)
			}
		})
	}
}

func TestRemuxFFmpegFormat(t *testing.T) {
	if got := RemuxFFmpegFormat(RemuxContainerMKV); got != "matroska" {
		t.Errorf("RemuxFFmpegFormat(mkv) = %q, want matroska (the muxer name)", got)
	}
	if got := RemuxFFmpegFormat(RemuxContainerMP4); got != "mp4" {
		t.Errorf("RemuxFFmpegFormat(mp4) = %q, want mp4", got)
	}
	// An empty container is a session predating the choice, or a reconstruct from
	// an older token. It must behave exactly as before.
	if got := RemuxFFmpegFormat(""); got != "mp4" {
		t.Errorf("RemuxFFmpegFormat(\"\") = %q, want mp4 (back-compat default)", got)
	}
	if got := RemuxFFmpegFormat("nonsense"); got != "mp4" {
		t.Errorf("RemuxFFmpegFormat(nonsense) = %q, want mp4", got)
	}
}

// Fragmentation is an MP4 workaround for writing to a pipe. Matroska streams as
// written, and carrying the flags over would misdescribe what we emit.
func TestBuildRemuxArgsFragmentsOnlyMP4(t *testing.T) {
	mp4 := strings.Join(buildRemuxArgs("/m.mkv", "mp4", 0, false, 0, 0, false, 2, 0, 0), " ")
	if !strings.Contains(mp4, "-movflags frag_keyframe+delay_moov+default_base_moof") {
		t.Errorf("MP4 must stay fragmented to stream over a pipe: %s", mp4)
	}
	if !strings.Contains(mp4, "-f mp4") {
		t.Errorf("want -f mp4: %s", mp4)
	}

	mkv := strings.Join(buildRemuxArgs("/m.mkv", "matroska", 0, false, 0, 0, false, 2, 0, 0), " ")
	if strings.Contains(mkv, "-movflags") {
		t.Errorf("Matroska needs no fragmentation flags: %s", mkv)
	}
	if !strings.Contains(mkv, "-f matroska") {
		t.Errorf("want -f matroska: %s", mkv)
	}
	// Both still end at the pipe -- the container choice does not change delivery.
	for _, args := range []string{mp4, mkv} {
		if !strings.HasSuffix(args, "pipe:1") {
			t.Errorf("output must remain pipe:1: %s", args)
		}
	}
}

func TestContainerMIMEMatroska(t *testing.T) {
	for _, name := range []string{"matroska", "mkv"} {
		if got := containerMIME(name); got != "video/x-matroska" {
			t.Errorf("containerMIME(%q) = %q, want video/x-matroska", name, got)
		}
	}
	if got := containerMIME("mp4"); got != "video/mp4" {
		t.Errorf("containerMIME(mp4) = %q, want video/mp4", got)
	}
}

// A piped remux cannot patch a duration in afterwards, so it has to declare the
// output length up front or the client gets no timeline and every seek fails.
func TestBuildRemuxArgsDeclaresDuration(t *testing.T) {
	args := func(seek, total, origin float64) string {
		return strings.Join(buildRemuxArgs("/m.mkv", "matroska", seek, false, 0, 0, false, 2, total, origin), " ")
	}

	// The margin keeps a duration stored as whole seconds from truncating the end.
	if got := args(0, 6227, 0); !strings.Contains(got, "-t 6229.000") {
		t.Errorf("want -t 6229.000 (6227 + margin), got: %s", got)
	}

	// Seeking shortens what remains to be written, measured from the timeline
	// origin -- the keyframe the copy seek lands on, not the requested position.
	if got := args(600, 6227, 600); !strings.Contains(got, "-t 5629.000") {
		t.Errorf("want -t 5629.000 (6227 - 600 + margin), got: %s", got)
	}

	// The case that matters: a keyframe gap far wider than the margin. Measuring
	// from the request (639) would declare 5590 for output that actually starts at
	// the keyframe (630) and so carries 5597 seconds -- a timeline nine seconds
	// short, whose tail the player could never reach.
	if got := args(639, 6227, 630); !strings.Contains(got, "-t 5599.000") {
		t.Errorf("want -t 5599.000 measured from the keyframe anchor, got: %s", got)
	}
	if got := args(639, 6227, 630); strings.Contains(got, "-t 5590.000") {
		t.Errorf("duration must not be measured from the requested seek: %s", got)
	}

	// An unknown duration must not produce a -t at all: capping the output at a
	// guessed length would truncate the stream.
	if got := args(0, 0, 0); strings.Contains(got, "-t ") {
		t.Errorf("unknown duration must not cap the output: %s", got)
	}

	// Nor when the seek is past the end -- a zero or negative -t would emit
	// nothing at all.
	if got := args(7000, 6227, 7000); strings.Contains(got, "-t ") {
		t.Errorf("seek past end must not cap the output: %s", got)
	}
}
