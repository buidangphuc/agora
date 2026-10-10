#!/usr/bin/env python3
# Usage (needs a GPU or patience): uv run --with "sentence-transformers==3.3.1" --with "torch==2.5.1" python scripts/semantic_floor_probe.py BAAI/bge-small-en-v1.5
# (torch 2.5.1 still ships sm_70 kernels, so it also runs on V100.)
"""Cosine distribution of related vs unrelated (query, listing title) pairs for a sentence embedding model."""
import sys, statistics
from sentence_transformers import SentenceTransformer
pairs_related = [
 ("tai nghe bluetooth", "Tai nghe Bluetooth chống ồn ANC pin 30 giờ"),
 ("tai nghe bluetooth", "Tai nghe không dây True Wireless Bluetooth 5.3"),
 ("sạc nhanh 65w", "Củ sạc nhanh GaN 65W 3 cổng USB-C"),
 ("áo thun nam", "Áo thun nam cotton cổ tròn form rộng"),
 ("giày chạy bộ", "Giày thể thao chạy bộ nam đế êm"),
 ("nồi chiên không dầu", "Nồi chiên không dầu 5 lít điện tử"),
 ("laptop gaming", "Laptop gaming RTX 4060 màn 144Hz"),
 ("son môi", "Son môi lì lâu trôi màu đỏ cam"),
 ("bluetooth headphones", "Wireless noise cancelling Bluetooth headphones"),
 ("fast charger 65w", "65W GaN USB-C fast charger"),
 ("running shoes", "Men's cushioned running shoes"),
 ("air fryer", "5 litre digital air fryer"),
 ("gaming laptop", "Gaming laptop RTX 4060 144Hz display"),
 ("lipstick", "Long lasting matte red lipstick"),
]
titles = [t for _, t in pairs_related]
queries = [q for q, _ in pairs_related]
unrelated = [(q, t) for i, q in enumerate(queries) for j, t in enumerate(titles)
             if i != j and queries[j] != q and not (set(q.lower().split()) & set(titles[j].lower().split()))]
nonsense = [("zxqv pltrk", t) for t in titles] + [("asdfgh qwerty", t) for t in titles]
name = sys.argv[1]
m = SentenceTransformer(name, device="cuda" if __import__("torch").cuda.is_available() else "cpu")
def cos(ps):
    a = m.encode([p[0] for p in ps], normalize_embeddings=True)
    b = m.encode([p[1] for p in ps], normalize_embeddings=True)
    return [float((x * y).sum()) for x, y in zip(a, b)]
def summ(label, xs):
    xs = sorted(xs)
    q = lambda p: xs[min(len(xs)-1, int(p*len(xs)))]
    print(f"{label:10s} n={len(xs):3d} min={xs[0]:.3f} p10={q(.1):.3f} median={statistics.median(xs):.3f} p90={q(.9):.3f} max={xs[-1]:.3f}")
vi = [p for p in pairs_related if any(c in p[1] for c in "ăâđêôơưáàảãạ")]
en = [p for p in pairs_related if p not in vi]
print("model", name)
summ("related", cos(pairs_related)); summ(" vi", cos(vi)); summ(" en", cos(en))
summ("unrelated", cos(unrelated)); summ("nonsense", cos(nonsense))
