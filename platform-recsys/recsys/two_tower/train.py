"""Train the two towers on (user, item) interactions with in-batch softmax.

Each tower is ``normalize(relu(x @ W + b))`` (see ``UserTower`` / ``ItemTower``). For a batch of
positive pairs the loss is the cross-entropy of every user against the batch's items (temperature
scaled cosine logits; other users' positives are the negatives, a repeat of the same item in the batch
is masked, not punished). Gradients are exact (ReLU and L2-normalisation are differentiated) and SGD
updates ``weights`` and ``bias`` of both towers in place. Pure numpy: it runs on the driver over the
sampled pairs, no Spark.
"""

from __future__ import annotations

import random
from collections.abc import Sequence
from typing import Any

import numpy as np

from recsys.two_tower.model import TwoTowerModel

_EPS = 1e-9


def _forward(x: np.ndarray, w: np.ndarray, b: np.ndarray):
    z = x @ w + b
    a = np.maximum(z, 0.0)
    norm = np.linalg.norm(a, axis=1, keepdims=True)
    e = np.where(norm > _EPS, a / np.maximum(norm, _EPS), 0.0)
    return z, e, norm


def _backward(de: np.ndarray, x: np.ndarray, z: np.ndarray, e: np.ndarray, norm: np.ndarray):
    # d normalize(a) / d a, applied to de; rows with a zero activation carry no gradient.
    da = np.where(norm > _EPS, (de - e * np.sum(de * e, axis=1, keepdims=True)) / np.maximum(norm, _EPS), 0.0)
    dz = da * (z > 0)
    return x.T @ dz, dz.sum(axis=0)


def train_towers(
    model: TwoTowerModel,
    pairs: Sequence[tuple[str, str]],
    user_features: dict[str, dict[str, Any]],
    item_features: dict[str, dict[str, Any]],
    *,
    epochs: int = 5,
    lr: float = 0.05,
    batch_size: int = 256,
    temperature: float = 0.1,
    seed: int = 42,
) -> list[float]:
    """Run ``epochs`` passes of SGD over ``pairs`` of (user_key, listing_id); returns the mean loss of
    each epoch. Pairs whose listing has no features row are dropped; a user without features trains as
    an all-zero input. With fewer than 2 usable pairs (or ``epochs`` 0) nothing is trained."""
    usable = [(u, i) for u, i in pairs if i in item_features]
    if epochs <= 0 or len(usable) < 2:
        return []

    ut, it = model.user_tower, model.item_tower
    wu, bu = np.array(ut.weights), np.array(ut.bias)
    wi, bi = np.array(it.weights), np.array(it.bias)
    ux = {u: np.array(ut._extract_input_vector(user_features.get(u, {}))) for u in {u for u, _ in usable}}
    ix = {i: np.array(it._extract_input_vector(item_features[i])) for i in {i for _, i in usable}}

    rng = random.Random(seed)
    order = list(usable)
    losses: list[float] = []
    for _ in range(epochs):
        rng.shuffle(order)
        total, batches = 0.0, 0
        for start in range(0, len(order), batch_size):
            batch = order[start : start + batch_size]
            if len(batch) < 2:
                continue
            xu = np.stack([ux[u] for u, _ in batch])
            xi = np.stack([ix[i] for _, i in batch])
            ids = np.array([i for _, i in batch])

            zu, eu, nu = _forward(xu, wu, bu)
            zi, ei, ni = _forward(xi, wi, bi)

            logits = (eu @ ei.T) / temperature
            same_item = ids[:, None] == ids[None, :]
            logits = np.where(same_item & ~np.eye(len(batch), dtype=bool), -1e9, logits)
            logits = logits - logits.max(axis=1, keepdims=True)
            probs = np.exp(logits)
            probs /= probs.sum(axis=1, keepdims=True)
            idx = np.arange(len(batch))
            total += float(-np.log(np.maximum(probs[idx, idx], 1e-12)).mean())
            batches += 1

            dlogits = probs.copy()
            dlogits[idx, idx] -= 1.0
            dlogits /= len(batch) * temperature
            dwu, dbu = _backward(dlogits @ ei, xu, zu, eu, nu)
            dwi, dbi = _backward(dlogits.T @ eu, xi, zi, ei, ni)
            wu -= lr * dwu
            bu -= lr * dbu
            wi -= lr * dwi
            bi -= lr * dbi
        if batches:
            losses.append(total / batches)

    ut.weights, ut.bias = wu.tolist(), bu.tolist()
    it.weights, it.bias = wi.tolist(), bi.tolist()
    return losses
