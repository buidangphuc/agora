# Bài học: đối chiếu track recsys-ML của Agora với một hệ recommender MLOps thật

> Tài liệu học tập, không phải kế hoạch thi công. Không có thay đổi code nào đi kèm.
>
> Đối tượng đối chiếu: [`Ecommerce-Recommender-System-On-AWS-With-MLOps`](https://github.com/nguyenthai-duong/Ecommerce-Recommender-System-On-Aws-With-Mlops)
> — Item2Vec + Ranking Sequence, Kubeflow/Ray/MLflow/Feast/KServe-Triton trên AWS.
>
> Ngày khảo sát: 2026-09-20. Mọi dẫn chứng `file:line` ở Phần 1 đã được mở ra kiểm chứng.

---

## Tóm tắt

Hai hệ giải cùng một bài toán ở hai tầng khác nhau. Repo AWS là **chiều sâu ML**: model
được huấn luyện thật, có vòng đời, có nơi phục vụ. Agora là **chiều rộng product platform**:
contract chặt, ranh giới domain rõ, gate spec→e2e nghiêm.

Điều đáng học nhất lại không nằm ở so sánh tính năng. Nó nằm ở chỗ này: `plan-mlops/INDEX.md`
tự đánh giá track là **13/13 task `done`, "MLOps Level 2"**. Đọc code thì phần lớn lớp ML là
**scaffolding chưa có call site production** — module có unit test xanh nhưng không ai gọi,
model mang tên thuật toán mà không thực hiện thuật toán đó, gate chất lượng chưa từng chạy
với số liệu thật.

Đây không phải lời chê. Đây là lớp lỗi phổ biến nhất khi dựng nền ML nhanh, và Agora **đã tự
phát hiện một phần** — bằng chứng là 4 OpenSpec change `wire-*` đang mở đúng cho lớp lỗi này.
Tài liệu này làm nốt việc: liệt kê đầy đủ, có bằng chứng, và đặt cạnh cách repo AWS làm thật
để thấy khoảng cách cụ thể là gì.

---

## Phần 1 — Thật vs giả trong Agora

### 1.1. Bốn module không có đường chạy

`recsys/pipeline.py` dòng 13-22 là danh sách đầy đủ những gì batch job chạm tới. Không có
trong danh sách đó nghĩa là không chạy.

| Module | Trạng thái |
|---|---|
| `recsys/registry/` | Không call site production. Chỉ `tests/test_registry.py` import |
| `recsys/ranker/` | Không call site production. Chỉ `tests/test_ranker.py` |
| `recsys/monitoring/drift.py` | Không call site production. Chỉ `tests/test_drift.py` |
| `recsys/nearline/` | Không call site production. Chỉ `tests/test_nearline.py` |
| `recsys/evals/` | Chỉ với tới được từ `registry/registry.py:8` (bản thân nó đã mồ côi) và từ `make eval` |

Hệ quả cụ thể: không có gì ghi một `ModelMetadata`; không có gì promote champion; không có gì
tính PSI trên một lần chạy thật; không có gì ghi key `recs:nearline:*`.

### 1.2. Two-tower sinh vector toàn số 0

Đây là lỗi nghiêm trọng nhất tìm được, vì nó im lặng.

`pipeline.py:89` dựng catalog:

```python
catalog_items = [{"listing_id": lid} for lid in item_ids]
```

Nhưng `ItemTower._extract_input_vector` (`two_tower/item_tower.py:42-55`) đọc
`category_id`, `price`, `historical_ctr`, `popularity_score` — **không trường nào có mặt**.
Nên input vector toàn 0. Rồi `project()` tính `bias[j] + 0` với `bias = [0.0]*embedding_dim`
(`item_tower.py:40`), ReLU ra 0, và guard `norm > 1e-9` bỏ qua bước chuẩn hóa.

Kết quả: **mọi listing nhận cùng một vector 0**, được upsert vào `item_two_tower_vectors`
với distance Cosine. Truy hồi trên collection đó vô nghĩa. Không có exception, không có log
cảnh báo — chỉ là kết quả rỗng nghĩa.

May là stage này gated sau `ENABLE_TWO_TOWER` (mặc định `false`), nên nó chưa gây hại. Nhưng
nó đang được tính là "done" trong bảng trạng thái.

### 1.3. Không có bước huấn luyện nào trong two-tower

Tên hàm là `train_and_index_two_tower` (`two_tower/pipeline.py:13-29`), nhưng thân hàm chỉ
construct `TwoTowerModel` rồi gọi `index_items`. Trọng số sinh bởi:

```python
rng = random.Random(seed + 100)
self.weights = [[rng.uniform(-0.1, 0.1) for _ in range(self.embedding_dim)]
                for _ in range(self.input_dim)]
self.bias = [0.0] * self.embedding_dim
```

và **không bao giờ được cập nhật**. Không có gradient step nào tồn tại trong repo.

### 1.4. `GBDTRanker` không phải GBDT

`ranker/model.py:15-25`: một tổng tuyến tính 7 trọng số hardcode, cộng một `if` thưởng 0.15.

```python
self.weights = feature_weights or [0.35, 0.20, 0.15, 0.05, 0.05, 0.10, 0.10]
```

Comment gọi đây là "Default learned weights" — chúng là hằng số viết tay. Không có cây, không
boosting, không `fit()`.

### 1.5. Hai bản scorer lệch nhau — train/serve skew đã xảy ra

Đây là thứ đáng lo nhất về mặt nguyên lý, vì nó là **đúng căn bệnh mà feature store sinh ra
để chữa**, và nó đang hiện diện ngay trong repo:

| | `platform-recsys/recsys/ranker/model.py:15` | `team-ai/.../ranking.py:85` |
|---|---|---|
| Trọng số | `[0.35, 0.20, 0.15, 0.05, 0.05, 0.10, 0.10]` | `[0.30, 0.25, 0.15, 0.05, 0.05, 0.10, 0.10]` |
| Boost rule | `vec[0]>0.7 and vec[1]>0.5 → +0.15` | `vec[1]>0.5 and vec[5]>0.05 → +0.20` |

Hai hàm chấm điểm độc lập, viết tay, lệch cả trọng số lẫn luật. Không bản nào nạp artifact đã
huấn luyện. Nếu bản offline từng được dùng để chọn model, thì model đó đang được phục vụ bằng
một hàm khác.

### 1.6. `platform-featurestore` không ai dùng

Grep toàn monorepo: **0 importer** ngoài chính nó. Không có trong `platform-gitops/`, không
có trong `requirements.txt`/`pyproject.toml` của repo nào, không có ArgoCD Application.
`Dockerfile` của nó có `CMD ["pytest", "-v", "tests/"]` — là image chạy test, không phải
image phục vụ.

Trớ trêu: docstring `ranker/model.py:34` nói feature được "enriched via feature store", nhưng
hàm nhận một `dict` thường và không import `featurestore`.

### 1.7. `OfflineFeatureStore` là hai dict in-memory

`offline.py:14-15`. Docstring của module nói "point-in-time queries"; code không có PIT join,
không có Parquet, không có warehouse. `build_training_dataset` (`offline.py:36-55`) join trên
**trạng thái hiện tại**.

Đây là điểm kỹ thuật quan trọng: một feature store không làm được point-in-time join thì
không giải quyết được vấn đề mà feature store tồn tại để giải quyết. Nó chỉ là một cache.

### 1.8. `make eval` là demo hardcode

`evals/__main__.py:14-29` chứa 6 interaction literal (`u1`/`item1`…) và một dict `predicted`
viết tay. Nó không đọc warehouse, không nạp model. Comment ghi "Predictions produced by a
trained model on training data" — không có model nào ở đây.

### 1.9. Promotion gate chưa từng chạy với số liệu thật

`evals/evaluator.py:113-149` có logic gate đúng: NDCG gain + coverage floor. Nhưng:

- Ngưỡng mặc định `min_relative_improvement=0.0` → **bất kỳ non-regression nào cũng promote**.
- `ModelMetadata.metrics` là dict mở, và **không có gì trong pipeline populate nó**.
- Không có tiêu chí latency, không có ý nghĩa thống kê, không có cỡ mẫu tối thiểu.

Một cổng so sánh hai dict rỗng với ngưỡng 0.0 luôn luôn mở.

### 1.10. `/rerank` lệch contract và hỏng im lặng

`platform-modelserve/modelserve/router.py:219` trả thẳng body của TEI — một **mảng JSON trần**
`[{index, score}]`.

Người tiêu thụ duy nhất, `team-search/internal/retrieval/rerank_client.go:50-52`, khai báo:

```go
type rerankResponse struct { Results []rerankResultItem `json:"results"` }
```

Một mảng trần sẽ unmarshal lỗi; một object thiếu key `results` cho `len == 0`, và client
**trả về `candidateIDs` chưa rerank** (`rerank_client.go:96-98`). Không test nào phủ seam này:
`test_router.py` assert với mock của chính router.

Nghĩa là: nếu bật rerank lên, nó sẽ "chạy" mà không rerank gì, và không ai biết.

### 1.11. CronJob nightly đọc thư mục rỗng

`platform-gitops/platform/recsys/cronjob.yaml:77-78` mount `/data` là `emptyDir`, trong khi
ConfigMap cùng file đặt `WAREHOUSE_DRIVER: "duckdb"` và
`WAREHOUSE_PARQUET_PATH: "/data/tracking_events.parquet"` (dòng 20-21). Job 03:00 UTC hằng
đêm tìm thấy một thư mục trống.

Chú thích ở dòng 19 cho thấy đây là thiết kế dở dang có ý thức — *"prod flips
WAREHOUSE_DRIVER=bigquery + the BIGQUERY_* coordinates"* — nghĩa là volume này chỉ dùng ở
local. Nhưng cấu hình đang commit là `duckdb`, nên ở trạng thái hiện tại job vẫn đọc rỗng.

### 1.12. Lệch tên biến môi trường giữa producer và consumer

- Producer `platform-recsys/recsys/config.py:105`: `RECS_SCHEMA_VERSION`
  (cũng ở `.env.example:53` và `cronjob.yaml:30`)
- Consumer `team-ai/app/core/config/recommendations.py:41`: `RECS_CACHE_SCHEMA_VERSION`

Cả hai cùng default `"v1"` nên hiện tại trùng nhau một cách may mắn. Đổi một bên sẽ làm
consumer đọc sai prefix key mà không có lỗi nào nổ ra.

### 1.13. Hai phần ba placement không với tới được qua gRPC

`config/placements.yaml` định nghĩa 3 placement: `home_feed`, `similar_items`,
`cart_cross_sell`. Nhưng proto không có trường `placement_id`, và servicer
(`transport/grpc/servicers/recommend.py:54-60`) không map `RecommendationContext` sang
placement. `service.py` không bao giờ đọc `query.context`.

Nên mọi RPC rơi về `placement_id="home_feed"`. `similar_items` và `cart_cross_sell` —
gồm cả ladder và ranking policy riêng của chúng — là code chết trên đường gRPC.

### 1.14. Provenance là chuỗi tĩnh

`RECS_MODEL_VERSION` mặc định `"serving-fallback"` (`recommendations.py:50`), được echo trên
mọi response. Trong khi đó `platform-recsys` đã ghi `recs:v1:model_version` vào Redis đúng
theo generation (`load/redis_cache.py:60`, ghi **sau** khi pipeline execute xong — thiết kế
tốt). team-ai không đọc key đó.

Nên trường `model_version` trong `RecommendResponse` — vốn có mục đích truy vết model nào tạo
ra ranking — không truy vết gì.

### 1.15. Ghi nhận công bằng: những thứ thật

Tài liệu chỉ ra chỗ giả thì cũng phải chỉ ra chỗ thật. Những thứ sau là code dùng được:

- **ALS Spark** (`train.py`) — model được huấn luyện thật, và là baseline duy nhất đang có.
- **Contract Qdrant/Redis** — chặt chẽ một cách đáng nể: point ID `uuid5` ổn định qua các lần
  chạy, prune theo generation để reader luôn thấy một thế hệ nhất quán, TTL 48h > nhịp nightly
  nên lỡ một lần chạy thì suy giảm chứ không rỗng, và key `model_version` được flip **cuối cùng**.
- **`platform-modelserve`** — admission control với 429 + `Retry-After`, Redis embedding cache
  keyed `hash(model_version + text)`, Prometheus metrics. Đây là phần chắc tay nhất của track.
- **`drift.py`** — PSI đúng công thức, ngưỡng 0.1/0.25 chuẩn ngành.
- **`evaluator.py`** — NDCG/Recall/Precision/MAP/MRR + temporal split viết đúng.
- **Bốn change `wire-*` đang mở** — Agora đã tự nhận diện lớp lỗi "module không ai gọi" và đặt
  tên cho nó. Đó là dấu hiệu tốt.

Điểm chung của nhóm này: **chúng là thư viện đúng, chỉ thiếu đường dây**.

---

## Phần 2 — Kỹ thuật ML: repo AWS làm thật ra sao

### 2.1. Item2Vec là SkipGram PyTorch thật

`src/model_item2vec/model.py`:

```python
class SkipGram(nn.Module):
    def __init__(self, num_items: int, embedding_dim: int):
        self.embeddings = nn.Embedding(num_items + 1, embedding_dim, padding_idx=num_items)
        nn.init.xavier_uniform_(self.embeddings.weight)
```

`forward` tính dot product giữa target và context embedding rồi qua sigmoid; dataset là
`SkipGramDataset` với negative sampling. Embedding được **học** từ chuỗi hành vi.

Đối chiếu thẳng với §1.2/§1.3: cùng mục tiêu (vector item cho ANN retrieval), một bên học từ
dữ liệu, một bên random-init rồi nhân với input rỗng.

### 2.2. Ranking Sequence là mô hình chuỗi thật, và nối với tầng retrieval

`src/model_ranking_sequence/model.py` — `class Ranker(nn.Module)`:

- `nn.Embedding` cho item, user, và **timestamp bucket** của chuỗi tương tác
- `nn.GRU` chạy trên chuỗi item lịch sử
- MLP `nn.Linear(...) → nn.Linear(embedding_dim, 1) → nn.Sigmoid()` cho điểm CTR
- `nn.Dropout` giữa các tầng

Quan trọng: constructor nhận `item_embedding: nn.Embedding = None` — *"pretrained item
embeddings"*. **Item2Vec feed thẳng vào ranker.** Hai tầng không chỉ tồn tại, chúng nối với nhau.

`trainer.py` là PyTorch Lightning thật: `torch.optim.Adam`, `ReduceLROnPlateau` monitor
`val_loss`, ROC-AUC tính cuối mỗi epoch, MLflow logger.

Đối chiếu §1.4: Agora gọi tổng tuyến tính 7 số là "stage-2 ranking". Khoảng cách không phải
là "model yếu hơn" — là **chưa có model**.

### 2.3. Có đường xuất artifact

`convert2onnx_and_build_triton.py` (31KB) — export sang ONNX và dựng Triton model repository.

Đây là khái niệm Agora **không có tương đương nào**. Trong Agora, "model" sống trong process
dưới dạng hằng số Python. Không có artifact, nên không có gì để version, để so sánh, để
rollback, để phục vụ ở nơi khác.

Bài học không phải "hãy dùng ONNX". Bài học là: **model phải là một artifact tách rời khỏi
code phục vụ nó**. Chừng nào trọng số còn nằm trong `__init__`, thì registry, promotion gate,
A/B, rollback đều không có đối tượng để thao tác — và đó chính xác là lý do §1.1 và §1.9 xảy ra.

### 2.4. Feast và point-in-time correctness

Feast giải quyết một vấn đề rất cụ thể: khi build tập huấn luyện từ lịch sử, giá trị feature
phải là giá trị **tại thời điểm đó**, không phải giá trị hiện tại. Nếu không, model học được
thông tin tương lai và đo rất đẹp lúc offline rồi sập lúc online.

`OfflineFeatureStore` của Agora (§1.7) join trên trạng thái hiện tại — đúng cái bẫy này. Và
vì nó không ai dùng (§1.6), bẫy chưa sập.

Điểm đáng học: tốt hơn là hai dict in-memory không được gọi, so với hai dict in-memory được
gọi và âm thầm rò rỉ label. Nhưng cả hai đều không phải feature store.

### 2.5. Train/serve skew là mối lo hạng nhất

Lý do Feast materialize feature ra online store, thay vì để mỗi bên tự tính: để **cùng một
định nghĩa feature** phục vụ cả training lẫn serving.

§1.5 là ca bệnh minh họa hoàn hảo — hai hàm chấm điểm lệch trọng số. Và vì không bên nào nạp
artifact, không có cách nào phát hiện lệch ngoài việc đọc chéo hai file.

`platform-featurestore` có `validate_parity` (`parity.py:30-36`) đúng ý tưởng: so sánh online
vs offline theo từng trường với dung sai. Nó chỉ chưa có dữ liệu thật để so.

### 2.6. IPS thật khác heuristic vị trí

`ranker/features.py:9-13` của Agora:

```python
def compute_ips_weight(position: int, gamma: float = 0.5) -> float:
    return float(math.pow(pos, gamma))
```

Đây là trọng số theo vị trí, không phải IPS. IPS là `1/propensity`, với propensity là **xác
suất item được hiển thị ở vị trí đó** — phải ước lượng từ dữ liệu, không phải một hàm mũ cố định.

Sự khác biệt quan trọng: IPS thật cần log đầy đủ về cái gì được hiển thị và ở đâu. Agora có
sẵn dữ liệu đó (`analytics.events` có `position`, `search_query`) nhưng chưa dùng để ước lượng
propensity.

---

## Phần 3 — MLOps / vận hành

### 3.1. MLflow ↔ `ModelMetadata` không ai ghi

ADR-0014 của Agora **cố ý từ chối** MLflow, gọi nó là over-engineered, và tự định nghĩa một
registry nhẹ trên Redis (`recs:model:champion`). Đó là một quyết định kiến trúc hợp lệ — không
phải mọi đội đều cần MLflow server.

Nên bài học ở đây **không phải "hãy dùng MLflow"**. Bài học là về *chức năng*, thứ độc lập
với công cụ:

| Câu hỏi | AWS repo | Agora |
|---|---|---|
| Ai ghi metrics? | `trainer.py` log mỗi epoch qua MLflow logger | Không ai |
| Ghi lúc nào? | Trong vòng train, tự động | — |
| Ai đọc để quyết định? | Model Promotion Watcher | `evaluate_and_promote` — chưa được gọi |
| Artifact ở đâu? | MLflow artifact store → Triton repo | Không có artifact |

Registry của Agora gọn hơn và phù hợp quy mô hơn. Nó chỉ thiếu ba thứ: người ghi, người gọi,
và một artifact để trỏ tới.

### 3.2. Kubeflow/Ray ↔ pipeline tuyến tính

Pipeline AWS là DAG có stage tách bạch (feature engineering → train → eval → cache → serve),
mỗi stage là một component có input/output rõ ràng.

`recsys/pipeline.py` là một hàm tuyến tính 70 dòng, và 4 module ML đứng hoàn toàn ngoài nó
(§1.1). Không phải vấn đề "thiếu Kubeflow" — một hàm tuyến tính hoàn toàn ổn ở quy mô này.
Vấn đề là **những stage đáng ra phải có trong đó thì không có**: không có stage eval, không
có stage register, không có stage drift.

### 3.3. Promotion Watcher ↔ gate chưa chạy

AWS: model mới được promote → watcher kích hoạt Jenkins pipeline → retrain/redeploy tự động.
Vòng lặp khép kín.

Agora có đủ mảnh (`evaluate_and_promote`, `CHAMPION_KEY`, atomic swap) nhưng không có ai gọi,
và không có metrics để so. Vòng lặp hở ở cả hai đầu.

### 3.4. KServe/Triton ↔ `platform-modelserve`

Đây là chỗ Agora làm **đúng về kiến trúc**: ADR-0011 đặt modelserve là platform capability
nội bộ, chỉ team-ai gọi, không qua gateway, "adopt vendor runtimes; write no inference code".
Vị trí này tương đương KServe trong hệ AWS, và nguyên tắc "không tự viết inference" là đúng.

Vấn đề là team-ai **có 0 reference** tới modelserve. `RAG_EMBED_SERVER_URL` rỗng ở mọi nơi.
`/rerank` hoàn toàn không có người tiêu thụ hoạt động (§1.10).

Một điểm kỹ thuật cần lưu ý nếu sau này nối thật: TEI là runtime cho embedding và cross-encoder.
Nó **không phục vụ được** một GBDT hay một GRU ranker. Muốn phục vụ ranker kiểu §2.2 thì cần
thêm một vendor runtime khác (ONNX Runtime / Triton) sau modelserve — nghĩa là ADR-0011 sẽ cần
bổ sung, chứ không phải chỉ cần nối dây.

### 3.5. Drift ↔ PSI không có run path

Code PSI của Agora đúng (§1.15). Thiếu: một baseline được lưu lại từ generation trước, và một
chỗ trong pipeline để so generation hiện tại với nó.

### 3.6. Locust ↔ đo latency thủng lỗ

`REQUEST_LATENCY` trong modelserve chỉ được observe ở **nhánh embed thành công**
(`router.py:173`). `/rerank` và `/generate` không ghi latency, và không nhánh lỗi nào ghi cả.

Nếu sau này muốn có tiêu chí latency trong promotion gate, đây là chỗ thủng.

---

## Phần 4 — Bảy bài học

Mỗi bài neo vào bằng chứng ở Phần 1.

**1. Unit test xanh không có nghĩa là đang chạy.**
Bốn module ở §1.1 đều có test đầy đủ và đều chưa từng chạy trong production. Định nghĩa "done"
cần bao gồm: *có call site trên đường chạy thật*. Agora đã tự rút ra bài này — bốn change
`wire-*` chính là nó.

**2. Đặt tên không tạo ra năng lực.**
Gọi một class là `GBDTRanker` không làm nó thành GBDT (§1.4). Gọi một hàm là
`train_and_index_two_tower` không làm nó huấn luyện (§1.3). Gọi hai dict là `OfflineFeatureStore`
không cho nó point-in-time join (§1.7). Từ vựng lạm phát nguy hiểm vì nó làm bảng trạng thái
trông đã xong.

**3. Một cổng không có số liệu là sân khấu.**
Ngưỡng 0.0 so sánh hai dict rỗng thì luôn mở (§1.9). Trước khi tin vào một promotion gate, hãy
hỏi: lần gần nhất nó **từ chối** một model là khi nào?

**4. Train/serve skew không phải rủi ro lý thuyết.**
Hai vector trọng số khác nhau trong cùng một repo (§1.5) là skew đã xảy ra rồi. Feature store
và artifact dùng chung tồn tại để chữa đúng bệnh này — và Agora có cả hai dưới dạng code chưa
nối.

**5. Contract phải được test ở seam, không phải ở mock của chính mình.**
`test_router.py` assert với mock của router và xanh; producer và consumer vẫn lệch shape
(§1.10). Test một phía không chứng minh được gì về giao điểm.

**6. Fallback im lặng che mất hỏng hóc.**
Rerank lỗi → trả list chưa rerank, không log, không metric (§1.10). Two-tower vector rỗng →
upsert bình thường, không exception (§1.2). Degradation tốt cần để lại dấu vết; nếu không,
nó chỉ là lỗi ẩn.

**7. Bảng trạng thái tự báo cáo sẽ trôi khỏi code.**
`INDEX.md` ghi 13/13 done vì mỗi task *đã được viết code*. Không có gate nào kiểm tra code đó
có được gọi không. Agora đã có tiền lệ cho cách chữa — gate `spec-check` buộc mỗi scenario
phải trỏ tới một e2e tự động — nhưng gate đó chưa áp cho lớp ML. Đáng chú ý: **0/33 change
trong track recommendations đã được archive**, nghĩa là chưa change nào đi qua gate đó.

---

## Phần 5 — Nếu muốn làm thật, thứ tự nào

Không phải lộ trình thi công. Chỉ là thứ tự phụ thuộc và lý do.

**Trước hết: làm cho đo được.**
`INDEX.md` đã tự đặt luật *"ALS không bị xoá cho tới P3-T3 thắng trên metric. Nó là baseline
duy nhất."* Luật đúng — nhưng hiện chưa có metric nào tồn tại. Nối `evals/` + `registry/` vào
`pipeline.py` để mỗi lần chạy sinh ra NDCG@10 thật cho ALS là việc đầu tiên, vì mọi phát biểu
"cải thiện" sau đó đều vô nghĩa nếu thiếu nó. Kèm theo: `make eval` phải đọc warehouse thay vì
6 dòng hardcode.

**Thứ hai: feature store, vì two-tower phụ thuộc nó.**
§1.2 không sửa được bằng cách vá `pipeline.py:89`. Gốc rễ là không có nguồn feature cho item.
`ItemFeatures` (`definitions.py:35-42`) đã là **superset đúng** của thứ `ItemTower` cần
(`category_id`, `price`, `historical_ctr`, `popularity_score`) và của `CandidateFeatures`.
Schema được thiết kế để khớp — chỉ thiếu dây. Nối được feature store thì two-tower mới có input
khác 0, và ranker mới có gì để chấm.

**Thứ ba: retrieval, rồi mới tới ranking.**
Theo đúng thứ tự của hệ AWS: embedding học trước (§2.1), rồi ranker dùng embedding đó làm
đầu vào (§2.2). Ngược lại thì ranker không có tín hiệu tốt để học.

**Song song, rẻ, nên làm sớm:** đọc `recs:v1:model_version` thay cho chuỗi tĩnh (§1.14) — nhỏ
nhưng là điều kiện cần để debug lệch offline/online về sau; map `RecommendationContext` →
`placement_id` trong servicer (§1.13) — mở khóa 2/3 placement mà không cần đổi proto; và sửa
seam `/rerank` (§1.10) trước khi có ai đó dựa vào nó.

**Cần quyết định kiến trúc trước khi tới ranking:** ADR-0011 nói không viết inference code, và
TEI không phục vụ được GBDT/GRU (§3.4). Nên hoặc thêm một vendor runtime sau modelserve, hoặc
giữ inference ở offline (precompute ranked list vào Redis như ALS đang làm). Lựa chọn thứ hai
hợp với ngân sách 15ms của `RECS_RETRIEVE_TIMEOUT_MS` hơn, và không cần sửa ADR.

---

## Phụ lục — Nguồn

**Agora** (khảo sát 2026-09-20, `~/Documents/agora`): `platform-recsys/recsys/{pipeline,config,
train,recommend}.py`, `recsys/{two_tower,ranker,registry,evals,monitoring,nearline,load}/`,
`team-ai/app/modules/business/recommend/{service,ranking,factory,backends,cache,placement_config}.py`,
`team-ai/app/transport/grpc/servicers/recommend.py`, `platform-modelserve/modelserve/{router,
admission,config}.py`, `platform-featurestore/featurestore/{online,offline,parity,definitions}.py`,
`team-search/internal/retrieval/rerank_client.go`, `team-gateway/internal/edge/recommendation.go`,
`platform-gitops/platform/recsys/cronjob.yaml`, `openspec/changes/`, `platform-e2e/scripts/
{features,spec_sync}.py`, `platform-core/docs/ADR/{0011,0012,0014}`, `plan-mlops/INDEX.md`.

**Repo AWS**: `src/model_item2vec/{model,trainer,dataset}.py`,
`src/model_ranking_sequence/{model,trainer,dataset}.py`,
`src/model_ranking_sequence/convert2onnx_and_build_triton.py`, README (kiến trúc tổng thể).
Các phát biểu về thuật toán ở Phần 2 dựa trên source, không dựa trên README.
