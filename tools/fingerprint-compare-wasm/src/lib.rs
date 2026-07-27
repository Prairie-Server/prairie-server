//! Chromaprint fingerprint comparison for intro marker detection.
//!
//! Port of `internal/intromarkers/compare.go` hot path. The Go layer still
//! handles chapter snapping, confidence scoring, and DB writes.

use std::collections::HashMap;

use serde::{Deserialize, Serialize};

pub const DEFAULT_POINT_HOP_SECONDS: f64 = 0.123;
const HAMMING_THRESHOLD: u32 = 6;
const MAX_SHIFT_CANDIDATES: usize = 8;
const MIN_SHIFT_VOTE_COUNT: i32 = 3;

#[derive(Debug, Clone, Deserialize)]
pub struct CompareRequest {
    #[serde(default = "default_point_hop")]
    pub point_hop_seconds: f64,
    pub minimum_intro_duration_seconds: i32,
    pub maximum_intro_duration_seconds: i32,
    pub inputs: Vec<FingerprintInput>,
}

fn default_point_hop() -> f64 {
    DEFAULT_POINT_HOP_SECONDS
}

#[derive(Debug, Clone, Deserialize)]
pub struct FingerprintInput {
    pub index: usize,
    pub episode_id: String,
    pub points: Vec<u32>,
}

#[derive(Debug, Clone, Serialize, PartialEq)]
pub struct Segment {
    pub start: f64,
    pub end: f64,
}

#[derive(Debug, Clone, Serialize, PartialEq)]
pub struct PairMatch {
    pub left_index: usize,
    pub right_index: usize,
    pub left: Segment,
    pub right: Segment,
}

#[derive(Debug, Clone, Serialize, PartialEq)]
pub struct CompareResponse {
    pub matches: Vec<PairMatch>,
}

pub fn compare_fingerprints(req: &CompareRequest) -> CompareResponse {
    let hop = if req.point_hop_seconds > 0.0 {
        req.point_hop_seconds
    } else {
        DEFAULT_POINT_HOP_SECONDS
    };
    let cfg = CompareConfig {
        point_hop_seconds: hop,
        minimum_intro_duration_seconds: req.minimum_intro_duration_seconds,
        maximum_intro_duration_seconds: req.maximum_intro_duration_seconds,
    };

    let mut matches = Vec::new();
    let inputs = &req.inputs;
    for i in 0..inputs.len() {
        for j in (i + 1)..inputs.len() {
            let left = &inputs[i];
            let right = &inputs[j];
            if left.episode_id.is_empty() || left.episode_id == right.episode_id {
                continue;
            }
            if let Some(pair) = compare_pair(&left.points, &right.points, &cfg) {
                matches.push(PairMatch {
                    left_index: left.index,
                    right_index: right.index,
                    left: pair.0,
                    right: pair.1,
                });
            }
        }
    }
    CompareResponse { matches }
}

#[derive(Debug, Clone)]
struct CompareConfig {
    point_hop_seconds: f64,
    minimum_intro_duration_seconds: i32,
    maximum_intro_duration_seconds: i32,
}

fn compare_pair(left: &[u32], right: &[u32], cfg: &CompareConfig) -> Option<(Segment, Segment)> {
    if left.is_empty() || right.is_empty() {
        return None;
    }

    let shifts = candidate_shifts(left, right, cfg.point_hop_seconds);
    let mut best_left = Segment {
        start: 0.0,
        end: 0.0,
    };
    let mut best_right = Segment {
        start: 0.0,
        end: 0.0,
    };
    let mut best_duration = 0.0;

    for shift in shifts {
        if let Some((left_seg, right_seg)) = compare_pair_at_shift(left, right, cfg, shift) {
            let duration = left_seg.end - left_seg.start;
            if duration > best_duration {
                best_left = left_seg;
                best_right = right_seg;
                best_duration = duration;
            }
        }
    }

    if best_duration == 0.0 {
        None
    } else {
        Some((best_left, best_right))
    }
}

fn compare_pair_at_shift(
    left: &[u32],
    right: &[u32],
    cfg: &CompareConfig,
    shift: i32,
) -> Option<(Segment, Segment)> {
    let tolerance = candidate_shift_sample_points(cfg.point_hop_seconds);
    let mut matches: Vec<(usize, usize)> = Vec::new();

    for (i, &lp) in left.iter().enumerate() {
        let center = i as i32 + shift;
        let from = 0.max(center - tolerance);
        let to = (right.len() as i32 - 1).min(center + tolerance);
        if to < from {
            continue;
        }
        for j in from..=to {
            if hamming_distance(lp, right[j as usize]) <= HAMMING_THRESHOLD {
                matches.push((i, j as usize));
                break;
            }
        }
    }

    if matches.is_empty() {
        return None;
    }

    let max_gap_points = (3.5 / cfg.point_hop_seconds).ceil() as i32;
    let mut best_start = 0usize;
    let mut best_end = 0usize;
    let mut best_run_start = 0usize;
    let mut run_start = 0usize;

    for i in 1..matches.len() {
        let left_gap = matches[i].0 as i32 - matches[i - 1].0 as i32;
        let right_gap = matches[i].1 as i32 - matches[i - 1].1 as i32;
        if left_gap <= max_gap_points && right_gap >= -tolerance && right_gap <= max_gap_points {
            continue;
        }
        if (matches[i - 1].0 - matches[run_start].0) > (best_end - best_start) {
            best_start = matches[run_start].0;
            best_end = matches[i - 1].0;
            best_run_start = run_start;
        }
        run_start = i;
    }
    if (matches[matches.len() - 1].0 - matches[run_start].0) > (best_end - best_start) {
        best_start = matches[run_start].0;
        best_end = matches[matches.len() - 1].0;
        best_run_start = run_start;
    }

    let start = best_start as f64 * cfg.point_hop_seconds;
    let end = (best_end + 1) as f64 * cfg.point_hop_seconds;
    let duration = end - start;
    if duration < cfg.minimum_intro_duration_seconds as f64
        || duration > cfg.maximum_intro_duration_seconds as f64
    {
        return None;
    }

    let right_shift = matches[best_run_start].1 as i32 - matches[best_run_start].0 as i32;
    let right_start = 0.max(best_start as i32 + right_shift) as f64 * cfg.point_hop_seconds;
    let right_end = right_start + duration;
    Some((
        Segment { start, end },
        Segment {
            start: right_start,
            end: right_end,
        },
    ))
}

fn candidate_shifts(left: &[u32], right: &[u32], point_hop_seconds: f64) -> Vec<i32> {
    let step = candidate_shift_sample_points(point_hop_seconds) as usize;
    let mut counts: HashMap<i32, i32> = HashMap::new();

    let mut i = 0;
    while i < left.len() {
        let mut j = 0;
        while j < right.len() {
            if hamming_distance(left[i], right[j]) <= HAMMING_THRESHOLD {
                *counts.entry(j as i32 - i as i32).or_default() += 1;
            }
            j += step;
        }
        i += step;
    }

    let mut candidates = vec![ShiftCandidate {
        shift: 0,
        count: *counts.get(&0).unwrap_or(&0),
    }];
    for (&shift, &count) in &counts {
        if shift == 0 || count < MIN_SHIFT_VOTE_COUNT {
            continue;
        }
        candidates.push(ShiftCandidate { shift, count });
    }

    candidates.sort_by(|a, b| {
        b.count
            .cmp(&a.count)
            .then_with(|| abs_i32(a.shift).cmp(&abs_i32(b.shift)))
            .then_with(|| a.shift.cmp(&b.shift))
    });

    let limit = MAX_SHIFT_CANDIDATES.min(candidates.len());
    let mut shifts = Vec::with_capacity(limit);
    let mut seen = HashMap::new();
    for candidate in candidates {
        if shifts.len() >= limit {
            break;
        }
        if seen.contains_key(&candidate.shift) {
            continue;
        }
        seen.insert(candidate.shift, ());
        shifts.push(candidate.shift);
    }
    shifts
}

#[derive(Debug, Clone)]
struct ShiftCandidate {
    shift: i32,
    count: i32,
}

fn candidate_shift_sample_points(point_hop_seconds: f64) -> i32 {
    1.max((1.0 / point_hop_seconds).round() as i32)
}

fn hamming_distance(a: u32, b: u32) -> u32 {
    (a ^ b).count_ones()
}

fn abs_i32(v: i32) -> i32 {
    if v < 0 {
        -v
    } else {
        v
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn make_cfg(min: i32, max: i32) -> CompareRequest {
        CompareRequest {
            point_hop_seconds: DEFAULT_POINT_HOP_SECONDS,
            minimum_intro_duration_seconds: min,
            maximum_intro_duration_seconds: max,
            inputs: Vec::new(),
        }
    }

    #[test]
    fn finds_shared_range() {
        let mut left = vec![0u32; 400];
        let mut right = vec![0u32; 400];
        for i in 0..400 {
            left[i] = (i + 1000) as u32;
            right[i] = (i + 5000) as u32;
        }
        for i in 40..300 {
            left[i] = i as u32;
            right[i] = i as u32;
        }

        let req = CompareRequest {
            point_hop_seconds: DEFAULT_POINT_HOP_SECONDS,
            minimum_intro_duration_seconds: 15,
            maximum_intro_duration_seconds: 120,
            inputs: vec![
                FingerprintInput {
                    index: 0,
                    episode_id: "ep1".into(),
                    points: left,
                },
                FingerprintInput {
                    index: 1,
                    episode_id: "ep2".into(),
                    points: right,
                },
            ],
        };

        let resp = compare_fingerprints(&req);
        assert_eq!(resp.matches.len(), 1);
        let m = &resp.matches[0];
        assert!(m.left.end - m.left.start >= 30.0);
    }

    #[test]
    fn skips_same_episode_pairs() {
        let points: Vec<u32> = (0..400).map(|i| i as u32).collect();
        let req = CompareRequest {
            point_hop_seconds: DEFAULT_POINT_HOP_SECONDS,
            minimum_intro_duration_seconds: 15,
            maximum_intro_duration_seconds: 120,
            inputs: vec![
                FingerprintInput {
                    index: 0,
                    episode_id: "ep1".into(),
                    points: points.clone(),
                },
                FingerprintInput {
                    index: 1,
                    episode_id: "ep1".into(),
                    points,
                },
            ],
        };
        assert!(compare_fingerprints(&req).matches.is_empty());
    }

    #[test]
    fn finds_shared_range_with_offset() {
        let mut left = vec![0u32; 700];
        let mut right = vec![0u32; 700];
        for i in 0..700 {
            left[i] = 0xAAAAAAAA ^ (i as u32);
            right[i] = 0x55555555 ^ (i as u32 * 3);
        }
        for i in 40..320 {
            let point = (i * 17 + 12345) as u32;
            left[i] = point;
            right[i + 160] = point;
        }

        let req = CompareRequest {
            point_hop_seconds: DEFAULT_POINT_HOP_SECONDS,
            minimum_intro_duration_seconds: 15,
            maximum_intro_duration_seconds: 120,
            inputs: vec![
                FingerprintInput {
                    index: 0,
                    episode_id: "ep1".into(),
                    points: left,
                },
                FingerprintInput {
                    index: 1,
                    episode_id: "ep2".into(),
                    points: right,
                },
            ],
        };

        let resp = compare_fingerprints(&req);
        assert_eq!(resp.matches.len(), 1);
        let m = &resp.matches[0];
        // Raw compare returns ~4.92s (index 40 * hop); Go adjustSegment snaps <=5s to 0.
        assert!(m.left.start >= 4.0 && m.left.start <= 5.5);
        assert!(m.right.start >= 23.0 && m.right.start <= 26.0);
        assert!(m.left.end - m.left.start >= 30.0);
    }

    #[test]
    fn compare_pair_at_shift_breaks_runs_on_backward_right_jump() {
        let mut left = vec![0u32; 160];
        let mut right = vec![0u32; 160];
        for i in 0..160 {
            left[i] = 0xAAAAAAAA ^ (i as u32 * 17);
            right[i] = 0x55555555 ^ (i as u32 * 31);
        }
        left[0] = 0x11111111;
        right[30] = 0x11111111;
        for i in 1..120 {
            let point = 0x22220000 + i as u32;
            left[i] = point;
            right[i - 1] = point;
        }

        let cfg = CompareConfig {
            point_hop_seconds: DEFAULT_POINT_HOP_SECONDS,
            minimum_intro_duration_seconds: 1,
            maximum_intro_duration_seconds: 120,
        };
        let (left_seg, _) = compare_pair_at_shift(&left, &right, &cfg, 0).unwrap();
        assert_ne!(left_seg.start, 0.0);
    }

    #[test]
    fn compare_pair_at_shift_allows_small_backward_jitter() {
        let mut left = vec![0u32; 180];
        let mut right = vec![0u32; 180];
        for i in 0..180 {
            left[i] = 0xAAAAAAAA ^ (i as u32 * 17);
            right[i] = 0x55555555 ^ (i as u32 * 31);
        }
        for i in 10..150 {
            let point = 0x33330000 + i as u32;
            left[i] = point;
            right[i] = point;
        }
        right[70] = 0x55555555;
        right[65] = left[71];

        let cfg = CompareConfig {
            point_hop_seconds: DEFAULT_POINT_HOP_SECONDS,
            minimum_intro_duration_seconds: 1,
            maximum_intro_duration_seconds: 120,
        };
        let (left_seg, _) = compare_pair_at_shift(&left, &right, &cfg, 0).unwrap();
        assert!(left_seg.end - left_seg.start >= 15.0);
    }

    #[test]
    fn candidate_shift_sample_points_matches_go() {
        assert_eq!(candidate_shift_sample_points(DEFAULT_POINT_HOP_SECONDS), 8);
    }

    #[test]
    fn empty_inputs_yield_no_matches() {
        let req = make_cfg(15, 120);
        assert!(compare_fingerprints(&req).matches.is_empty());
    }
}
