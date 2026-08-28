#!/usr/bin/env python3
"""
Benchmark embedding models for Prairie recommendations.

Extracts media items from the dev database, embeds them with multiple models,
and compares similarity quality using metadata as proxy ground truth.

Usage:
    python3 scripts/benchmark-embeddings.py \
        --db-host localhost \
        --models lmstudio:text-embedding-qwen3-embedding-0.6b=http://localhost:1234 \
                 openai:text-embedding-3-large \
        --openai-key sk-... \
        --sample-size 500
"""

import argparse
import json
import math
import os
import subprocess
import sys
import time
from collections import defaultdict
from dataclasses import dataclass, field
from urllib.request import Request, urlopen
from urllib.error import HTTPError

# ---------------------------------------------------------------------------
# Data extraction — single query with lateral join for credits
# ---------------------------------------------------------------------------

SAMPLE_SQL = """
SELECT json_agg(t)::text FROM (
  SELECT
    mi.content_id,
    mi.title,
    mi.type,
    mi.year,
    mi.genres,
    mi.overview,
    mi.content_rating,
    mi.tagline,
    mi.studios,
    mi.networks,
    mi.countries,
    mi.keywords,
    mi.original_language,
    COALESCE(cr.credits, '[]'::json) AS credits
  FROM media_items mi
  LEFT JOIN LATERAL (
    SELECT json_agg(json_build_object(
      'name', p.name,
      'kind', ip.kind,
      'character', ip.character,
      'sort_order', ip.sort_order
    ) ORDER BY ip.sort_order) AS credits
    FROM item_people ip
    JOIN people p ON p.id = ip.person_id
    WHERE ip.content_id = mi.content_id
      AND (ip.kind IN (2, 3) OR (ip.kind = 1 AND ip.sort_order <= 5))
  ) cr ON true
  WHERE mi.status = 'matched'
    AND mi.overview IS NOT NULL
    AND mi.overview != ''
    AND array_length(mi.genres, 1) > 0
  ORDER BY random()
  LIMIT {limit}
) t;
"""


def run_psql(db_host: str, sql: str) -> str:
    """Execute SQL via SSH + docker exec, piping via stdin to avoid escaping."""
    ssh_cmd = "docker exec -i postgres psql -U prairie -d prairie -At"
    cmd = ["ssh", f"root@{db_host}", ssh_cmd]
    result = subprocess.run(cmd, capture_output=True, text=True, timeout=120, input=sql)
    if result.returncode != 0:
        print(f"psql error: {result.stderr}", file=sys.stderr)
        sys.exit(1)
    return result.stdout.strip()


def fetch_sample(db_host: str, sample_size: int) -> list[dict]:
    print(f"Fetching {sample_size} items with credits from {db_host}...")
    raw = run_psql(db_host, SAMPLE_SQL.format(limit=sample_size))
    items = json.loads(raw)

    with_credits = sum(1 for it in items if it.get("credits") and len(it["credits"]) > 0)
    print(f"  Got {len(items)} items ({with_credits} with cast/crew)")
    return items


# ---------------------------------------------------------------------------
# Text construction — mirrors BuildEmbeddingText() in
# internal/recommendations/embeddings/text.go exactly
# ---------------------------------------------------------------------------

def build_embedding_text(item: dict) -> str:
    parts = []
    genres = item.get("genres") or []
    overview = item.get("overview") or ""
    type_name = "TV series" if item.get("type") == "series" else "movie"

    # Semantic lead: genres + type + overview (overview truncated to 1000 runes)
    if genres and overview:
        # Python slice on str counts code points, matching Go's rune semantics
        trunc = overview[:1000]
        parts.append(f"{', '.join(genres)} {type_name} about {trunc}")
    elif genres:
        parts.append(f"{', '.join(genres)} {type_name}")
    elif overview:
        parts.append(f"{type_name}. {overview[:1000]}")

    # Title with year
    year = item.get("year") or 0
    if year > 0:
        parts.append(f"{item['title']} ({year})")
    else:
        parts.append(item["title"])

    if item.get("content_rating"):
        parts.append(f"Rated {item['content_rating']}")

    if item.get("tagline"):
        parts.append(f'"{item["tagline"]}"')

    # Cast: up to 5 actors (kind=1) with character names
    credits = item.get("credits") or []
    actors = [c for c in credits if c["kind"] == 1][:5]
    if actors:
        cast_parts = []
        for a in actors:
            if a.get("character"):
                cast_parts.append(f"{a['name']} as {a['character']}")
            else:
                cast_parts.append(a["name"])
        parts.append(f"Cast: {', '.join(cast_parts)}")

    # Directors (kind=2)
    directors = [c["name"] for c in credits if c["kind"] == 2]
    if directors:
        parts.append(f"Directed by {', '.join(directors)}")

    writers = [c["name"] for c in credits if c["kind"] == 3]
    if writers:
        parts.append(f"Written by {', '.join(writers)}")

    if item.get("keywords"):
        parts.append(f"Keywords: {', '.join(item['keywords'][:5])}")

    if item.get("original_language"):
        parts.append(f"Original language: {item['original_language']}")

    if item.get("studios"):
        parts.append(f"Studios: {', '.join(item['studios'])}")

    if item.get("networks"):
        parts.append(f"Network: {', '.join(item['networks'])}")

    countries = item.get("countries") or []
    if countries:
        parts.append(f"Country: {', '.join(countries[:2])}")

    return ". ".join(parts)


# ---------------------------------------------------------------------------
# Embedding API clients
# ---------------------------------------------------------------------------

def embed_openai_compatible(texts: list[str], model: str, base_url: str,
                            api_key: str = "", batch_size: int = 10) -> list[list[float]]:
    all_embeddings = [None] * len(texts)
    for i in range(0, len(texts), batch_size):
        batch = texts[i:i + batch_size]
        payload = json.dumps({"model": model, "input": batch}).encode()
        headers = {"Content-Type": "application/json"}
        if api_key:
            headers["Authorization"] = f"Bearer {api_key}"

        req = Request(f"{base_url}/v1/embeddings", data=payload, headers=headers)
        for attempt in range(5):
            try:
                with urlopen(req, timeout=120) as resp:
                    data = json.loads(resp.read())
                break
            except HTTPError as e:
                body = e.read().decode("utf-8", errors="replace")[:200]
                if e.code == 429 or e.code >= 500:
                    wait = min(2 ** attempt, 30)
                    print(f"    Retry {attempt + 1} after {wait}s ({e.code})...")
                    time.sleep(wait)
                elif e.code == 400:
                    print(f"\n    ERROR 400 from {base_url}: {body}", file=sys.stderr)
                    print(f"    Model '{model}' may not support embeddings in this server.", file=sys.stderr)
                    return None
                else:
                    raise
            except Exception as e:
                wait = min(2 ** attempt, 30)
                print(f"    Retry {attempt + 1} after {wait}s ({e})...")
                time.sleep(wait)
        else:
            print(f"    FAILED batch starting at index {i}", file=sys.stderr)
            return None

        for d in data["data"]:
            all_embeddings[i + d["index"]] = d["embedding"]

        done = min(i + batch_size, len(texts))
        print(f"    Embedded {done}/{len(texts)}", end="\r")

    print()
    return all_embeddings


def embed_gemini(texts: list[str], model: str, api_key: str,
                 batch_size: int = 10, task_type: str = "") -> list[list[float]]:
    all_embeddings = []
    for i in range(0, len(texts), batch_size):
        batch = texts[i:i + batch_size]
        url = f"https://generativelanguage.googleapis.com/v1beta/models/{model}:batchEmbedContents?key={api_key}"
        requests_body = []
        for t in batch:
            req = {"model": f"models/{model}", "content": {"parts": [{"text": t}]}}
            if task_type:
                req["taskType"] = task_type
            requests_body.append(req)
        payload = json.dumps({"requests": requests_body}).encode()
        headers = {"Content-Type": "application/json"}

        req = Request(url, data=payload, headers=headers)
        for attempt in range(5):
            try:
                with urlopen(req, timeout=120) as resp:
                    data = json.loads(resp.read())
                break
            except HTTPError as e:
                if e.code == 429 or e.code >= 500:
                    wait = min(2 ** attempt, 30)
                    print(f"    Retry {attempt + 1} after {wait}s ({e.code})...")
                    time.sleep(wait)
                else:
                    raise
        else:
            print(f"    FAILED batch at index {i}", file=sys.stderr)
            sys.exit(1)

        for emb in data["embeddings"]:
            all_embeddings.append(emb["values"])

        done = min(i + batch_size, len(texts))
        print(f"    Embedded {done}/{len(texts)}", end="\r")

    print()
    return all_embeddings


# ---------------------------------------------------------------------------
# Similarity / evaluation
# ---------------------------------------------------------------------------

def cosine_sim(a: list[float], b: list[float]) -> float:
    dot = sum(x * y for x, y in zip(a, b))
    na = math.sqrt(sum(x * x for x in a))
    nb = math.sqrt(sum(x * x for x in b))
    if na == 0 or nb == 0:
        return 0.0
    return dot / (na * nb)


def jaccard(a: set, b: set) -> float:
    if not a and not b:
        return 0.0
    union = len(a | b)
    return len(a & b) / union if union else 0.0


@dataclass
class ItemMeta:
    content_id: str
    title: str
    genres: set = field(default_factory=set)
    directors: set = field(default_factory=set)
    actors: set = field(default_factory=set)
    studios: set = field(default_factory=set)


def metadata_similarity(a: ItemMeta, b: ItemMeta) -> dict:
    return {
        "genre_jaccard": jaccard(a.genres, b.genres),
        "shared_directors": len(a.directors & b.directors),
        "shared_actors": len(a.actors & b.actors),
        "shared_studios": len(a.studios & b.studios),
        "genre_overlap": len(a.genres & b.genres),
    }


def metadata_relevance_score(meta_sim: dict) -> float:
    score = 0.0
    score += meta_sim["genre_jaccard"] * 0.4
    score += min(meta_sim["shared_directors"], 1) * 0.3
    score += min(meta_sim["shared_actors"] / 2, 1) * 0.2
    score += min(meta_sim["shared_studios"], 1) * 0.1
    return score


def evaluate_model(name: str, embeddings: list[list[float]], items_meta: list[ItemMeta],
                   top_k: int = 10) -> dict:
    n = len(embeddings)

    # Use numpy if available for large matrices, else fall back to pure Python
    try:
        import numpy as np
        print(f"  Computing {n}x{n} similarity matrix (numpy)...")
        emb_array = np.array(embeddings, dtype=np.float32)
        norms = np.linalg.norm(emb_array, axis=1, keepdims=True)
        norms[norms == 0] = 1.0
        normed = emb_array / norms
        sim_matrix = (normed @ normed.T).tolist()
    except ImportError:
        print(f"  Computing {n}x{n} similarity matrix (pure python — install numpy for speed)...")
        sim_matrix = [[0.0] * n for _ in range(n)]
        for i in range(n):
            for j in range(i + 1, n):
                s = cosine_sim(embeddings[i], embeddings[j])
                sim_matrix[i][j] = s
                sim_matrix[j][i] = s

    genre_overlaps = []
    director_hits = []
    actor_hits = []
    relevance_at_k = []
    ndcg_scores = []

    for i in range(n):
        neighbors = sorted(range(n), key=lambda j: sim_matrix[i][j], reverse=True)
        neighbors = [j for j in neighbors if j != i][:top_k]

        item_genre_overlaps = []
        item_director_hits = 0
        item_actor_hits = 0
        item_relevance = []

        for rank, j in enumerate(neighbors):
            ms = metadata_similarity(items_meta[i], items_meta[j])
            item_genre_overlaps.append(ms["genre_overlap"])
            item_director_hits += ms["shared_directors"] > 0
            item_actor_hits += ms["shared_actors"] > 0
            item_relevance.append(metadata_relevance_score(ms))

        genre_overlaps.append(sum(item_genre_overlaps) / len(item_genre_overlaps) if item_genre_overlaps else 0)
        director_hits.append(item_director_hits)
        actor_hits.append(item_actor_hits)
        relevance_at_k.append(sum(item_relevance) / len(item_relevance) if item_relevance else 0)

        # NDCG: does the embedding ranking match the ideal metadata ranking?
        all_relevance = []
        for j in range(n):
            if j == i:
                continue
            all_relevance.append(metadata_relevance_score(metadata_similarity(items_meta[i], items_meta[j])))

        ideal = sorted(all_relevance, reverse=True)[:top_k]
        actual = item_relevance

        dcg = sum(r / math.log2(k + 2) for k, r in enumerate(actual))
        idcg = sum(r / math.log2(k + 2) for k, r in enumerate(ideal))
        ndcg_scores.append(dcg / idcg if idcg > 0 else 0.0)

    return {
        "model": name,
        "dimensions": len(embeddings[0]),
        "avg_genre_overlap_at_k": sum(genre_overlaps) / n,
        "avg_director_hits_at_k": sum(director_hits) / n,
        "avg_actor_hits_at_k": sum(actor_hits) / n,
        "mean_relevance_at_k": sum(relevance_at_k) / n,
        "mean_ndcg_at_k": sum(ndcg_scores) / n,
    }


def print_topk_examples(name: str, embeddings: list[list[float]], items: list[dict],
                        items_meta: list[ItemMeta], num_examples: int = 5, top_k: int = 5):
    n = len(embeddings)
    print(f"\n{'='*80}")
    print(f"  Example recommendations: {name}")
    print(f"{'='*80}")

    indices = list(range(min(num_examples, n)))

    for i in indices:
        sims = [(j, cosine_sim(embeddings[i], embeddings[j])) for j in range(n) if j != i]
        sims.sort(key=lambda x: x[1], reverse=True)

        genres_str = ', '.join(items[i].get('genres') or [])
        print(f"\n  {items[i]['title']} ({items[i].get('year', '?')}) [{genres_str}]")
        print(f"  {'─' * 70}")
        for rank, (j, sim) in enumerate(sims[:top_k]):
            ms = metadata_similarity(items_meta[i], items_meta[j])
            shared = []
            if ms["genre_overlap"]:
                shared.append(f"{ms['genre_overlap']}g")
            if ms["shared_directors"]:
                shared.append(f"{ms['shared_directors']}d")
            if ms["shared_actors"]:
                shared.append(f"{ms['shared_actors']}a")
            tag = f" [{','.join(shared)}]" if shared else ""
            j_genres = ', '.join(items[j].get('genres') or [])
            print(f"    {rank+1}. {items[j]['title']} ({items[j].get('year', '?')}) "
                  f"[{j_genres}] sim={sim:.3f}{tag}")


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def parse_model_spec(spec: str) -> dict:
    """Parse model spec. Supports optional +task_type suffix for Gemini.
    Examples:
        gemini:gemini-embedding-2-preview+SEMANTIC_SIMILARITY
        openai:text-embedding-3-large
        lmstudio:model-name=http://host:port
    """
    if ":" not in spec:
        raise ValueError(f"Invalid model spec: {spec}. Format: provider:model-name[+task_type][=base_url]")

    provider, rest = spec.split(":", 1)
    task_type = ""

    if "=" in rest:
        model_part, base_url = rest.split("=", 1)
    else:
        model_part = rest
        base_url = None

    if "+" in model_part:
        model, task_type = model_part.split("+", 1)
    else:
        model = model_part

    if provider == "openai" and not base_url:
        base_url = "https://api.openai.com"

    return {"provider": provider, "model": model, "base_url": base_url, "task_type": task_type}


def main():
    parser = argparse.ArgumentParser(description="Benchmark embedding models for Prairie")
    parser.add_argument("--db-host", default="localhost", help="Dev server SSH host")
    parser.add_argument("--models", nargs="+", required=True,
                        help="Model specs: provider:model[=base_url]. "
                             "Providers: lmstudio, openai, gemini")
    parser.add_argument("--openai-key", default=os.environ.get("OPENAI_API_KEY", ""))
    parser.add_argument("--gemini-key", default=os.environ.get("GEMINI_API_KEY", ""))
    parser.add_argument("--sample-size", type=int, default=500)
    parser.add_argument("--top-k", type=int, default=10, help="Number of neighbors to evaluate")
    parser.add_argument("--examples", type=int, default=5, help="Number of example items to show")
    parser.add_argument("--batch-size", type=int, default=20)
    parser.add_argument("--cache-dir", default="/tmp/prairie-embed-bench",
                        help="Cache embeddings to avoid re-computing")
    parser.add_argument("--no-cache", action="store_true", help="Ignore cached data")
    args = parser.parse_args()

    os.makedirs(args.cache_dir, exist_ok=True)

    # 1. Fetch data
    cache_file = os.path.join(args.cache_dir, f"sample_{args.sample_size}.json")
    if os.path.exists(cache_file) and not args.no_cache:
        print(f"Loading cached sample from {cache_file}")
        with open(cache_file) as f:
            items = json.load(f)
    else:
        items = fetch_sample(args.db_host, args.sample_size)
        with open(cache_file, "w") as f:
            json.dump(items, f)

    # 2. Build embedding texts (identical to Go's BuildEmbeddingText)
    texts = [build_embedding_text(item) for item in items]
    avg_len = sum(len(t) for t in texts) / len(texts)
    print(f"\nEmbedding texts: {len(texts)} items, avg {avg_len:.0f} chars")

    # Spot-check: show a sample text
    print(f"\n  Sample text ({items[0]['title']}):")
    print(f"  {texts[0][:200]}...")

    # 3. Build metadata for ground truth
    items_meta = []
    for item in items:
        credits = item.get("credits") or []
        m = ItemMeta(
            content_id=item["content_id"],
            title=item["title"],
            genres=set(item.get("genres") or []),
            directors={c["name"] for c in credits if c["kind"] == 2},
            actors={c["name"] for c in credits if c["kind"] == 1},
            studios=set(item.get("studios") or []),
        )
        items_meta.append(m)

    has_directors = sum(1 for m in items_meta if m.directors)
    has_actors = sum(1 for m in items_meta if m.actors)
    print(f"  Metadata: {has_actors} items with actors, {has_directors} with directors")

    # 4. Embed with each model
    model_specs = [parse_model_spec(s) for s in args.models]
    results = {}

    for spec in model_specs:
        task_suffix = f"+{spec['task_type']}" if spec.get('task_type') else ""
        label = f"{spec['provider']}:{spec['model']}{task_suffix}"
        cache_key = f"{spec['model']}{task_suffix}".replace("/", "_")
        emb_cache = os.path.join(args.cache_dir, f"embeddings_{cache_key}_{args.sample_size}.json")

        if os.path.exists(emb_cache) and not args.no_cache:
            print(f"\n[{label}] Loading cached embeddings...")
            with open(emb_cache) as f:
                embeddings = json.load(f)
        else:
            print(f"\n[{label}] Embedding {len(texts)} items...")
            t0 = time.time()

            if spec["provider"] in ("lmstudio", "openai", "ollama"):
                api_key = args.openai_key if spec["provider"] == "openai" else ""
                embeddings = embed_openai_compatible(
                    texts, spec["model"], spec["base_url"],
                    api_key=api_key, batch_size=args.batch_size,
                )
            elif spec["provider"] == "gemini":
                embeddings = embed_gemini(
                    texts, spec["model"], args.gemini_key,
                    batch_size=args.batch_size,
                    task_type=spec.get("task_type", ""),
                )
            else:
                print(f"Unknown provider: {spec['provider']}", file=sys.stderr)
                sys.exit(1)

            if embeddings is None:
                print(f"  SKIPPING {label} (embedding failed)")
                continue

            elapsed = time.time() - t0
            print(f"  Done in {elapsed:.1f}s ({len(texts)/elapsed:.1f} items/sec, {len(embeddings[0])} dims)")

            with open(emb_cache, "w") as f:
                json.dump(embeddings, f)

        # 5. Evaluate
        print(f"\n[{label}] Evaluating top-{args.top_k} quality...")
        result = evaluate_model(label, embeddings, items_meta, top_k=args.top_k)
        results[label] = result

        print_topk_examples(label, embeddings, items, items_meta,
                            num_examples=args.examples, top_k=5)

    # 6. Summary
    print(f"\n{'='*80}")
    print(f"  BENCHMARK RESULTS (top-{args.top_k} neighbors, {len(items)} items)")
    print(f"{'='*80}")

    header = f"{'Model':<45} {'Dims':>5} {'NDCG@K':>7} {'Rel@K':>7} {'Genre':>6} {'Dir':>5} {'Actor':>5}"
    print(f"\n{header}")
    print("─" * len(header))

    sorted_results = sorted(results.values(), key=lambda r: r["mean_ndcg_at_k"], reverse=True)
    for r in sorted_results:
        print(f"{r['model']:<45} {r['dimensions']:>5} "
              f"{r['mean_ndcg_at_k']:>7.3f} "
              f"{r['mean_relevance_at_k']:>7.3f} "
              f"{r['avg_genre_overlap_at_k']:>6.2f} "
              f"{r['avg_director_hits_at_k']:>5.2f} "
              f"{r['avg_actor_hits_at_k']:>5.2f}")

    best = sorted_results[0]
    print(f"\nBest model by NDCG@{args.top_k}: {best['model']}")

    if len(sorted_results) >= 2:
        print(f"\nPairwise deltas vs {best['model']}:")
        for r in sorted_results[1:]:
            ndcg_diff = best["mean_ndcg_at_k"] - r["mean_ndcg_at_k"]
            rel_diff = best["mean_relevance_at_k"] - r["mean_relevance_at_k"]
            print(f"  vs {r['model']}: NDCG {'+' if ndcg_diff >= 0 else ''}{ndcg_diff:.3f}, "
                  f"Rel {'+' if rel_diff >= 0 else ''}{rel_diff:.3f}")


if __name__ == "__main__":
    main()
