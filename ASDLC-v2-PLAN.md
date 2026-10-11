# Agora ASDLC v2 — Master Plan

> File này dành cho Claude Code. Đặt ở root của Agora, đọc cùng `AGENTS.md`.
> Mỗi wave được chạy bằng skill `parallel-scrum` (sinh `plan/` + task packet), sau đó `scripts/validate_plan.py`.
> Bản v2.1 (2026-10-09): đã sửa theo review đối chiếu repo thật. Xem mục 13 (workflow initiative) và mục 14 (thí nghiệm Kev).

## 0. Cách dùng với Claude Code

1. Đọc `AGENTS.md`, file này, rồi recon repo (read-only). Mọi đường dẫn trong mục 7 là **đề xuất**: nếu repo đã có chỗ tương đương thì **mở rộng chỗ đó** và ghi lại, không viết script mới song song.
2. Giải quyết các mục `[UNRESOLVED]` ở mục 10 trước khi fan-out wave liên quan.
3. **Thứ tự chạy wave: W0 → W3 (thí nghiệm Kev, trừ W3-4) → W1 → W2 → W3-4 → W4 → W5.** W3 không phụ thuộc W1/W2 vì eval set dựng từ lịch sử git. Trong một wave, task có write-set rời nhau thì chạy song song được.
4. Mỗi task xong phải chạy `Verify` và được một agent khác review (không tự chấm).
5. Không push, không mở PR, không xóa dữ liệu, không đụng hạ tầng thật khi chưa có human duyệt. Task nào có Verify cần "PR mẫu" thì đó là **điểm duyệt human**.
6. Cập nhật cột `Status` ở mục 7 sau mỗi task (`todo | running | review | done | failed`).
7. **Mục tiêu PoC: show code + flow. Mọi thứ tạm chạy local** (CI chạy bằng `act` hoặc gọi thẳng lệnh, deploy lên kind local, Kev trên DGX gọi từ Mac).

---

## 1. Hiện trạng (mình có gì)

Số liệu recon ngày 2026-10-09; recon lại 2026-10-11 trên `main`: 69 spec, 1 change mở, 86 archived, 209 `.feature`, 15 ADR, 19 FEATURES.yaml.

| Nhóm | Có |
|---|---|
| Repo | **Một git repo duy nhất** (`buidangphuc/agora`, public). Các `team-*` / `platform-*` là thư mục con, không phải repo riêng |
| Spec | OpenSpec 1.12, **một `openspec/` ở cấp cả agora**: 52 capability spec, 20 change đang mở, 51 archived, `openspec validate --strict`. `openspec/ARCHIVE-STATUS.md` đã có nhưng stale (2026-09-04) |
| Ownership | `FEATURES.yaml` ở từng repo (19 file). Repo giữ `FEATURES.yaml` của capability là chủ capability đó |
| Traceability | 188 `.feature`, `make -C platform-e2e features-check` (`features.py --strict`) / `spec-check` (`spec_sync.py`) |
| Test | E2E black-box trên stack thật, parallel lane `-n 4` + serial lane, flake protocol |
| Guardrail | 11 skill, 2 reviewer (`contract-boundary`, `auth-scope`), 3 hook (`guard-generated`, `format`, `drift-check`), `repo_doctor.py`, `validate_plan.py` (có `--audit` kiểm write-set), 15 ADR |
| Planning | `plans/<initiative>/` (00-spec.md, PROGRAM.md, task packet theo wave) do `parallel-scrum` sinh; `spec-dispatch` build một change (code track + e2e track) |
| CI | **Chưa có `.github/workflows/` ở root.** `ci.yaml` trong `team-ai/`, `platform-recsys/`, `platform-modelserve/` không chạy (GitHub chỉ đọc workflow ở root) |
| Release / Operate | `deploy/` = local GitOps plane (kind + ArgoCD + Gitea + registry + ingress, `make -C deploy cluster-up / smoke`). `platform-gitops/` = Helm chart chung, overlay staging/prod, Argo Rollouts canary có auto-rollback theo Prometheus, Vault/ExternalSecrets, backup CronJob, Prometheus. Spec `deploy-runtime`, `observability`, `backup-dr`, `ops-cockpit`; ADR-0004 |
| Parallelism | Worktree (của một repo), proto đi tuần tự, compose project riêng |
| Decision engine | **Kev-27B** (fp16, GPU 0-2 trên DGX `osg-dgx1`, port 8012, tmux `kev27-8012`) + client `~/tools/kev/kev.py` trên Mac (tự mở SSH tunnel, trả 1 dòng JSON), `~/tools/kev/KEV.md`. GPU 3-7 trống |

## 2. Chuẩn SDLC và khoảng trống

| Phase | Chuẩn cần | Đang thiếu |
|---|---|---|
| 1 Intake / Planning | Ưu tiên, WIP limit, chia initiative theo team | WIP limit, cập nhật `ARCHIVE-STATUS.md`, khâu initiative → change theo team (mục 13) |
| 2 Specify | Functional + NFR + data classification + threat model | NFR, data classification, threat model, spec quality gate, review chéo giữa các change |
| 3 Plan | Write-set rời, contract trước | (đã có) |
| 4 Build | TDD, người viết test ≠ người viết code | Tách vai trò, khóa file test, vòng worker → reviewer |
| 5 Verify | Gate độc lập | Workflow CI ở root, security scan, red-first proof, mức test tối thiểu theo loại scenario, mutation test |
| 6 Review | Reviewer độc lập, quyết định có luật | feature-reviewer, privacy-reviewer, decision engine (Kev) đã đo hiệu quả |
| 7 Integrate | Một fix = một commit | (đã có) |
| 8 Release | Staging/prod, rollback | Đã có GitOps local. Thiếu: overlay staging chạy thật trên kind, thử rollback thật 1 lần |
| 9 Operate | Health, alert, runbook | Đã có Prometheus + health probe. Thiếu: alert rule bắn ra kênh (local), runbook |
| 10 Learn | Học từ production + incident | Đầu vào từ incident/metric; human duyệt thay đổi luật |
| Cross-cutting | Compliance dữ liệu người dùng, quyền agent, đo lường | Privacy checklist, audit log, baseline metric |

## 3. Quyết định đã chốt

| Chủ đề | Quyết định |
|---|---|
| Mục tiêu | PoC chứng minh ASDLC cho công ty: show code + flow, tạm chạy local |
| Phạm vi | Đủ 10 phase, bản mỏng nhất chạy được, đào sâu khi có nhu cầu thật |
| Solo / team | Một chuẩn, hai profile (`solo` / `team`) trong `asdlc.config.yaml`; chỉ đổi độ nghiêm ngặt, không đổi quy trình. "team" là số người tham gia, không phải cách chia repo |
| Spec | **Giữ OpenSpec, một `openspec/` cho cả agora**, ownership theo capability (repo giữ `FEATURES.yaml`). Thêm spec quality gate (ý tưởng từ Spec Kit clarify/checklist/analyze + EARS của Kiro), không đổi framework |
| Initiative | Plan tổng ở `plans/<id>/` → chia slice, mỗi slice một OpenSpec change cho team chủ → orchestrator review spec từng change và chéo giữa các change → human khóa → build theo thứ tự phụ thuộc (mục 13) |
| Build | TDD hai vòng (e2e ngoài, unit/integration trong). Người viết test ≠ người viết code. Vòng orchestrator → worker → reviewer cho mỗi feature |
| Superpowers | Chỉ dùng skill TDD / debugging / worktree bên trong worker. Không dùng brainstorm/plan (OpenSpec + `parallel-scrum` làm việc đó) |
| Nguồn sự thật gate | File workflow `.github/workflows/asdlc-gates.yml`. **Tạm chạy local** (`act` hoặc gọi thẳng lệnh). E2E vẫn là gate local (full stack không vừa runner CI) |
| Decision engine | **Kev local trên DGX**, chọn vì chi phí và tính tất định (xác suất có thể calibrate), không phải vì bảo mật code (repo public, worker là Claude). Script trả lời câu chắc chắn (routing, write-set, secret, buf); Kev trả lời câu phán đoán có đáp án định sẵn; LLM reviewer cho vùng không chắc; human cho rủi ro cao |
| Bộ câu hỏi | Lấy từ chuẩn có sẵn, không tự nghĩ ra; mỗi câu có trường `source:` trỏ tới ID gốc (bảng ở mục 6) |
| Luật quyết định | Veto → chặn. Thường → đạt khi vượt ngưỡng. Vùng không chắc → LLM. Rủi ro cao → human. **Không lấy trung bình** |
| Kev trong CI | **Không.** DGX chỉ để chạy model Kev. Kev chỉ chạy local qua `decide.py`, chỉ tư vấn (không chặn) cho tới khi thí nghiệm mục 14 cho số đủ tốt; bảng quyết định dán vào PR. Không dùng self-hosted runner (repo public, PR từ fork chạy code tùy ý trên runner) |
| Deploy | **Dùng GitOps local có sẵn**: kind + ArgoCD + Helm (`deploy/`, `platform-gitops/`). Giống stack công ty (ArgoCD + Helm). Staging = overlay trên kind local, rollback qua Argo Rollouts |
| Compliance | PDPA Việt Nam (NĐ 13/2023), OWASP ASVS L1-2, PCI DSS (không tự xử lý thẻ), nguyên tắc GDPR (minimization, retention). PoC chỉ dùng dữ liệu giả |
| Bảo mật agent | Agent không giữ credential prod; nội dung đọc vào là dữ liệu, không phải lệnh; log Kev lưu state đầy đủ ở local (gitignored, dữ liệu giả) để chạy lại khi sửa câu hỏi |
| Chỉ số chứng minh | Thời gian giao hàng (cycle time), số bug lọt, chi phí (token + GPU); baseline lấy từ git log **trước** khi đổi quy trình |
| Không làm lúc này | VPS / cloud thật, observability stack nặng, dashboard đẹp, approval matrix phức tạp, BMAD, Kiro IDE, self-hosted runner |

## 4. Lifecycle v2

```
1 Intake/Initiative → 2 Specify (team, song song) ─► [Spec gate: từng change + chéo] → 3 Plan
→ 4 Build (per slice, TDD, theo DAG) → 5 Verify (CI local) → 6 Review ─► [Feature gate]
→ 7 Integrate → 8 Release ─► [Release gate] → 9 Operate → 10 Learn
```

| Phase | Artifact | Exit gate |
|---|---|---|
| 1 Initiative | `plans/<id>/00-spec.md` + `slices.yaml` (slice, owner, depends_on, interface) | `validate_plan.py`; DAG không vòng |
| 2 Specify | `openspec/changes/<slice>/` có NFR, data classification, threat model ngắn; scenario theo EARS; `initiative: <id>` trong proposal | `openspec validate --strict` + `spec_lint` (chỉ delta) + `cross_change_check` + Spec gate (Kev) + human khóa |
| 3 Plan | Task tách code track / e2e track | `validate_plan.py` |
| 4 Build | e2e đỏ → (unit đỏ → code → xanh) mỗi task → e2e xanh | Task gate sau mỗi task |
| 5 Verify | CI xanh (chạy local) | Các check ở W1 |
| 6 Review | Bảng quyết định theo từng entry FEATURES.yaml | Feature gate, không còn veto |
| 8 Release | Image tag, sync overlay staging trên kind, smoke | Rollback đã thử thật 1 lần |
| 9 Operate | Health, log, 3 alert, runbook | Alert bắn được ra kênh local |
| 10 Learn | Đề xuất diff `AGENTS.md` / ADR từ incident | Human duyệt |

## 5. Vai trò

| Vai trò | Loại | Thấy | Không được |
|---|---|---|---|
| Orchestrator | Phiên chính | Initiative, change, FEATURES.yaml, kết quả worker, bảng quyết định | Tự viết code |
| `team-spec-author` | Subagent (1 mỗi slice) | Initiative + repo chủ của slice + proto | Ghi ngoài `openspec/changes/<slice>/`; thấy code repo khác |
| `spec-quality-reviewer` | Subagent | Spec | Thấy code |
| `e2e-author` | Subagent | Spec + FEATURES.yaml | Thấy code |
| `test-author` | Subagent | Spec + interface/proto | Thấy implementation |
| `implementer` | Subagent (1 worktree mỗi slice) | Test + code | Sửa file test (hook chặn) |
| `task-reviewer` | Subagent | Diff của task + scenario | Sửa code |
| `feature-reviewer` | Subagent | FEATURES.yaml + spec + test + bảng Kev | Sửa code |
| `privacy-reviewer` | Subagent | Data classification + diff | Sửa code |
| `flake-triager`, `incident-triager`, `ml-metrics-auditor` | Subagent | Log / metric | Sửa code |
| `contract-boundary-reviewer`, `auth-scope-reviewer` | Subagent (đã có) | Diff | Chỉ được gọi khi script routing (path glob) định tuyến tới |
| `initiative-plan`, `initiative-build`, `feature-run`, `spec-clarify`, `release` | Skill | — | — |
| `decide.py`, `spec_lint.py`, `cross_change_check.py`, `red_first.sh`, `kev_eval/` | Script | — | — |

Lưu ý: subagent không gọi được subagent khác, nên orchestrator luôn là phiên chính (skill) hoặc một Workflow script. Nhiều initiative song song = nhiều phiên Claude Code, mỗi phiên một worktree. "Team" trong PoC = một subagent `team-spec-author` cho mỗi repo chủ; ở công ty là người thật + phiên Claude của họ. Artifact bàn giao giống nhau (thư mục change).

## 6. Decision engine (Kev)

**Luồng mỗi gate:** script → Kev (bộ câu hỏi có version) → `decide.py` áp luật → vùng không chắc sang LLM reviewer → rủi ro cao sang human → bảng quyết định vào PR.

**Chia việc:** câu kiểm được bằng máy (routing auth/proto/PII bằng path glob, sửa ngoài write-set bằng `validate_plan.py --audit`, secret, `buf breaking`, có file test) → **script**. Chỉ câu cần phán đoán mới tới Kev.

**Format bộ câu hỏi** (`gates/<gate>.v<N>.yaml`):

```yaml
gate: feature
version: 1
state_unit: scenario          # orchestrator cắt state theo từng scenario, giữ < 8k token
questions:
  - id: checks_store
    source: "ISO/IEC/IEEE 29119-4 §5.2 (ví dụ, ghi đúng mục khi viết)"
    type: noul                # noul | choice | score
    severity: veto            # veto | normal
    instructions: "Does the test verify the persisted state the scenario promises, not only the HTTP response?"
    pass_when: true
    threshold: 0.85           # đặt từ đường precision/recall trên eval set (mục 14)
  - id: assertion_kind
    source: "Meszaros xUnit Test Patterns / tsDetect test smells"
    type: choice
    severity: veto
    instructions: "What do the assertions mainly check?"
    criteria:                 # thứ tự cố định, luôn có mô tả
      real: "Asserts persisted state or observable behaviour the spec promises"
      proxy: "Asserts only a status code or something incidentally true"
      mock: "Asserts that a mock was called"
    pass_values: [real]
    min_confidence: 0.6
```

**Ngữ nghĩa ngưỡng (noul):** gọi p = P(đáp án = `pass_when`). p ≥ `threshold` → pass; p ≤ 1 − `threshold` → fail; ở giữa → không chắc. Với `choice`: confidence của đáp án top < `min_confidence` → không chắc.

**Luật trong `decide.py`:**

| Trường hợp | Kết quả |
|---|---|
| Câu `veto` fail | `BLOCK` (giai đoạn advisory: ghi `WOULD_BLOCK`, không chặn) |
| Mọi câu pass | `PASS` |
| Có câu không chắc | `ESCALATE_LLM` cho đúng câu đó |
| Change đụng auth / payment / PII / release prod (script phát hiện) | `NEEDS_HUMAN` dù pass |
| Kev không phản hồi | `ENGINE_DOWN` → fallback LLM reviewer, ghi rõ trong bảng |

**Chuẩn nguồn cho từng gate:**

| Gate | Chuẩn nguồn | Ví dụ câu |
|---|---|---|
| Spec | ISO/IEC/IEEE 29148 (unambiguous, complete, singular, verifiable, feasible…) + mẫu EARS | Điều kiện kích hoạt rõ? Kết quả đo được? Có case lỗi? Có case quyền? |
| Task / Feature | ISO/IEC/IEEE 29119-4 (equivalence, boundary, error guessing) + catalog test smell (Meszaros / tsDetect) | Code làm nhiều hơn test yêu cầu? Test kiểm hành vi thật hay mock? Có case biên? Assertion khớp lời hứa spec (score 0-3)? |
| Code review | Google Engineering Practices "What to look for" | Thiết kế có hợp với hệ thống? Có phức tạp thừa? |
| Security | OWASP ASVS L1-2 (ghim một version: 4.0.3 hoặc 5.0, trích đúng requirement ID) + OWASP Top 10 | Có kiểm quyền sở hữu tài nguyên? |
| Privacy | NĐ 13/2023 (mục đích, đồng ý, tối thiểu hóa, lưu trữ) | Thu dữ liệu cá nhân ngoài mục đích đã nêu? |
| Release | Production Readiness Review (Google SRE) | Migration đảo ngược được? Breaking change có trong release note? |

## 7. Waves và task

Cột `Writes` là write-set độc quyền (đề xuất, xác nhận lại khi recon). `Verify` là lệnh/điều kiện kiểm chứng.

### Wave 0 — Nền móng (chạy solo)

| ID | Task | Writes | Verify / Done | Status |
|---|---|---|---|---|
| W0-1 | Viết `docs/ASDLC-v2.md`: 10 phase, exit gate, 2 profile, luật quyết định, workflow initiative (rút từ mục 3-6, 13) | `docs/ASDLC-v2.md` | Link từ `AGENTS.md`; một subagent mới chỉ đọc doc trả lời đúng 5/5 câu kiểm tra soạn sẵn | done |
| W0-2 | `asdlc.config.yaml` với `profile: solo|team` và các tham số (WIP, approval, reviewer bắt buộc, ngưỡng) | `asdlc.config.yaml`, `scripts/asdlc_config.py` | Có schema; `scripts/asdlc_config.py --check` pass | done |
| W0-3 | Mở rộng template OpenSpec: mục NFR, Data classification (PDPA/ASVS), Threat model; scenario theo EARS | `openspec/config.yaml`, template của OpenSpec trong repo | `openspec validate --strict` vẫn pass với change mẫu và với 20 change đang mở | done |
| W0-4 | Khung thư mục: `gates/`, `scripts/asdlc/`, `.claude/agents/` mới (file có frontmatter, chưa bật) | các thư mục đó | Tồn tại, không phá skill cũ | done |
| W0-5 | **Baseline từ git log**: cycle time (proposal → archive) và số commit fix sau merge của 51 change đã archive | `scripts/asdlc/baseline.py`, `docs/baseline.md` | Có bảng số; lệnh chạy lại được | done |
| W0-S1 | **Spike Kev latency** (quan trọng nhất: quyết định Kev có rẻ hơn LLM không): đo 1 câu ngắn, state ~4k token, N câu trên cùng state (prefix cache); thử cài `causal_conv1d` + `flash-linear-attention` trên V100 (sm70), timebox 1 ngày | Kết quả ghi vào mục 11 | Có số latency đơn câu / nhiều câu; kết luận cài được kernel hay không ("không được" là kết quả chấp nhận) | blocked (Kev down, GPU bận) |
| W0-S3 | **Spike hook theo từng subagent**: xác minh hook có biết subagent nào đang gọi (trường trong hook input, hoặc hook khai báo trong frontmatter subagent) | Ghi vào `docs/ASDLC-v2.md` | Có cách chặn implementer sửa test mà không chặn test-author | done |

(W0-S2 fp16 vs fp32: **bỏ.** 27B fp32 ≈ 108 GB không vừa thoải mái 4×32 GB V100, và calibrate ngưỡng trên eval set ở mục 14 đã hấp thụ lệch fp16.)

### Wave 1 — Verify backbone (CI, chạy local)

| ID | Task | Writes | Verify / Done | Status |
|---|---|---|---|---|
| W1-1 | `spec_lint.py`: EARS lint + checklist ISO 29148 (có NFR, data classification, case lỗi, case quyền, kết quả đo được). **Chỉ lint delta spec** của change (`openspec/changes/<id>/specs`) | `scripts/asdlc/spec_lint.py` + test | Spec thiếu mục → exit ≠ 0; chạy trên 3 change đang mở, liệt kê lỗi | done |
| W1-2 | Mở rộng `spec_sync.py`: tasks.md ↔ FEATURES.yaml ↔ `.feature` (thiếu, trùng, mâu thuẫn) | `platform-e2e/scripts/spec_sync.py` + test | Lệch → exit ≠ 0; `make spec-check` cũ vẫn chạy | done |
| W1-3 | Mở rộng `features.py`: mức test tối thiểu theo loại scenario (mục 8). Tag theo `<capability>/<requirement>` (không theo change id, vì gãy khi archive) | `platform-e2e/scripts/features.py` + test | Scenario thiếu loại test bắt buộc → exit ≠ 0 | done |
| W1-4 | `red_first.sh`: unit/integration chạy test mới trên `merge-base` (phải đỏ) và `HEAD` (phải xanh). E2E: kiểm thứ tự commit (commit test trước commit impl) | `scripts/asdlc/red_first.sh` | Test viết sau code bị bắt | done |
| W1-5 | Lint chặn mock của chính hệ thống trong e2e/integration | `scripts/asdlc/no_self_mock.py` | Có mock tự thân → exit ≠ 0 | done |
| W1-6 | **Tích hợp (cuối wave):** `.github/workflows/asdlc-gates.yml`: features-check, spec-check, repo_doctor, buf lint/breaking, gitleaks, trivy, semgrep, W1-1..W1-5, các job đang chết trong `team-ai/`, `platform-recsys/`, `platform-modelserve/` đưa lên root. `claude-code-action /security-review` để optional (cần API key) | `.github/workflows/asdlc-gates.yml` | Chạy local bằng `act` (hoặc từng lệnh) xanh; secret giả bị chặn. Push + bật required check là điểm duyệt human | review (local xanh; push = human) |

### Wave 2 — Build theo TDD + vòng reviewer + workflow initiative

| ID | Task | Writes | Verify / Done | Status |
|---|---|---|---|---|
| W2-1 | Subagent `team-spec-author`, `e2e-author`, `test-author`, `implementer` (prompt + tool giới hạn + chỉ đọc đúng phạm vi) | `.claude/agents/{team-spec-author,e2e-author,test-author,implementer}.md` | Chạy thử trên 1 task cũ | done |
| W2-2 | Subagent `task-reviewer`, `feature-reviewer`, `spec-quality-reviewer`, `privacy-reviewer` | `.claude/agents/{task,feature,spec-quality,privacy}-reviewer.md` | Feature-reviewer xuất bảng pass/fail theo FEATURES.yaml | done |
| W2-3 | Hook `guard-tests.sh` (theo cách đã chốt ở W0-S3): implementer không được sửa `*_test.go`, `*.spec.ts`, `tests/`, `*.feature` | `.claude/hooks/guard-tests.sh` + đăng ký | Implementer sửa test → bị chặn; test-author không bị chặn | done |
| W2-4 | Skill `spec-clarify`: hỏi human các chỗ mơ hồ ảnh hưởng e2e trước khi khóa spec | `.claude/skills/spec-clarify/` | Chạy trên 1 change, ra danh sách câu hỏi | done |
| W2-5 | Cấu hình Superpowers chỉ bật TDD / debugging / worktree; ghi phạm vi vào `AGENTS.md` | `.claude/settings` (phần plugin), mục mới trong `AGENTS.md` | Không có hai quy trình planning cạnh tranh | done (plugin cài scope project; phạm vi trong AGENTS.md) |
| W2-6 | `cross_change_check.py` (mở rộng `spec_sync.py` / `repo_doctor.py`): RPC/event mà slice dùng có trong contract slice; mỗi scenario thuộc đúng 1 FEATURES.yaml; không 2 slice sửa cùng requirement; DAG `depends_on` không vòng | `scripts/asdlc/cross_change_check.py` + test | Mỗi loại lệch → exit ≠ 0 | done |
| W2-7 | Skill `feature-run`: orchestrator chạy e2e-author → (test-author → implementer → task-reviewer)* → feature-reviewer; tối đa 3 vòng mỗi task rồi escalate | `.claude/skills/feature-run/` | Chạy trọn 1 feature cũ, chỉ dừng ở điểm duyệt | review (chưa chạy feature thật) |
| W2-8 | **Tích hợp (cuối wave):** skill `initiative-plan` (bước 1-3 mục 13) + `initiative-build` (bước 4-5). Tái dùng `parallel-scrum`, `spec-dispatch`, `feature-run`, `validate_plan.py` | `.claude/skills/initiative-plan/`, `.claude/skills/initiative-build/` | Chạy lại `plans/backend` qua bước 1-3, so slice sinh ra với các change đã thực sự làm; dừng đúng ở điểm human khóa spec | review (replay bước 1-2) |

### Wave 3 — Decision engine (Kev) = thí nghiệm A/B/C (mục 14)

W3-1..W3-3, W3-6 chạy ngay sau W0. W3-4 chờ W2-2.

| ID | Task | Writes | Verify / Done | Status |
|---|---|---|---|---|
| W3-1 | `kev.py`: thêm `--questions <file>`, nhiều câu trên một state (prefix cache), log `~/tools/kev/log.jsonl` (state đầy đủ local + hash, câu hỏi, xác suất, model, latency), timeout, exit code rõ, báo `ENGINE_DOWN` | `~/tools/kev/kev.py`, `~/tools/kev/KEV.md` (ngoài repo) | Mọi lần gọi có log; tắt server → exit code riêng | done |
| W3-2 | Bộ câu hỏi lấy từ chuẩn (mục 6): `gates/feature.v1.yaml` (ưu tiên), sau đó `spec.v1`, `task.v1`, `release.v1`. Routing **không** dùng Kev (script) | `gates/*.yaml` | Mỗi câu có id, `source`, type, severity, mô tả option, ngưỡng | done |
| W3-3 | `decide.py`: cắt state theo `state_unit`, gọi Kev (adapter TypeSafe-compatible, đổi engine không đổi câu hỏi), áp luật mục 6, xuất `decisions.json` + bảng markdown | `scripts/asdlc/decide.py` + test (mock engine) | Cùng input → cùng quyết định; test đủ 5 trường hợp của bảng luật | done |
| W3-6 | **`kev_eval/`**: build eval set (diff thật từ change đã archive + lỗi cài gắn ID chuẩn + nhóm lỗi ngoài checklist), chạy nhánh A/B/C, nhánh phụ Kev-4B (GPU 3-7), xuất bảng so sánh + đường precision/recall để đặt ngưỡng | `scripts/asdlc/kev_eval/`, `metrics/kev-eval/` | Bảng A/B/C theo từng câu; ngưỡng trong `gates/*.yaml` cập nhật từ số đo; kết luận câu nào giao Kev, câu nào giữ LLM | review (harness + set thật; chờ Kev) |
| W3-4 | Gắn `decide.py` vào `feature-reviewer`, `task-reviewer`, `spec-quality-reviewer` ở chế độ **advisory** (`WOULD_BLOCK`), routing reviewer chuyên bằng script | prompt các reviewer (đã tạo ở W2) | Feature mẫu có bảng quyết định; reviewer chuyên chỉ chạy khi được định tuyến | done |
| W3-5 | Vận hành Kev: thay tmux bằng systemd/supervisor, health check `/v1/models` | `server/` trong `~/tools/kev/` + DGX | Server tự lên lại sau reboot (cần human duyệt khi đụng DGX) | blocked (đụng DGX = human) |

### Wave 4 — Release và Operate (trên GitOps local có sẵn)

| ID | Task | Writes | Verify / Done | Status |
|---|---|---|---|---|
| W4-1 | Build image có tag commit cho mọi service, bump tag trong `platform-gitops` (dùng lại `make -C deploy images`) | `deploy/images/`, `platform-gitops/envs/` | Image có tag commit; ArgoCD sync đúng tag | done (live: 12 image tag 01a53993082a) |
| W4-2 | Overlay staging chạy trên kind local (ApplicationSet có sẵn) + smoke | `platform-gitops/envs/staging/`, `deploy/Makefile` | `make -C deploy smoke` xanh trên overlay staging | done (live: smoke-staging 8/12; 4 fail = cùng lỗi backend như ns marketplace 7/12) |
| W4-3 | Rollback: thử thật 1 lần qua Argo Rollouts (canary fail → auto-rollback) và lệnh rollback tay 1 dòng | `scripts/rollback.sh`, ghi kết quả vào `docs/runbooks/rollback.md` | Rollback đã thử thật 1 lần, có log | done (live: canary auto-abort ~100s + rollback --to) |
| W4-4 | Operate tối thiểu: 3 alert rule Prometheus (error rate, latency, service down) bắn ra kênh local (webhook/log giả), 1 runbook mỗi service | `platform-gitops/platform/monitoring/`, `docs/runbooks/` | Alert thử bắn được ra kênh local | done (live: alert route tới webhook-logger) |
| W4-5 | Subagent `incident-triager` + skill `release` | `.claude/agents/incident-triager.md`, `.claude/skills/release/` | Chạy thử trên 1 sự cố giả | done |

### Wave 5 — Đo lường, chứng minh, chuẩn bị scale

| ID | Task | Writes | Verify / Done | Status |
|---|---|---|---|---|
| W5-1 | Thu metric: cycle time, bug lọt sau merge, số vòng worker-reviewer, token, latency Kev, số lần gate đỏ | `scripts/asdlc/metrics.py`, `metrics/` | Có số liệu từ ≥3 feature, so với baseline W0-5 | done |
| W5-4 | Mutation test trên file thay đổi (Go/TS/Python), ngưỡng thấp rồi nâng dần | job mới trong `asdlc-gates.yml` | Có mutation score cho change mẫu | review (job chưa chạy) |
| W5-5 | Thử CodeGraph trên 2-3 repo (xác nhận hỗ trợ Go) | `docs/experiments/codegraph.md` | Có số token tiết kiệm được hoặc kết luận bỏ | done |
| W5-7 | Scale-up kit: `CODEOWNERS` theo capability (`openspec/specs/<capability>/` → team chủ FEATURES.yaml), approval theo risk tier, onboarding 1 trang, WIP limit, **cập nhật** `openspec/ARCHIVE-STATUS.md` | `CODEOWNERS`, `docs/ONBOARDING.md`, `openspec/ARCHIVE-STATUS.md` | Một subagent mới chỉ đọc onboarding làm được 1 change nhỏ qua đủ gate | review |

(W5-2 baseline → chuyển thành W0-5. W5-3 shadow mode và W5-6 Kev-4B → gộp vào W3-6. W5-8 self-hosted runner → **bỏ**.)

## 8. Test tối thiểu theo loại scenario

| Loại scenario | Test bắt buộc |
|---|---|
| Hành vi người dùng | 1 e2e + unit cho logic chính |
| Proto / contract | Contract test consumer ↔ provider + `buf breaking` |
| Quyền / bảo mật | E2E probe tấn công (user A đụng tài nguyên user B) |
| Logic có bất biến (ví, giá, tồn kho) | Unit + property test |
| Kafka / outbox | Integration với broker thật: phát và tiêu thụ event |
| Pipeline ML | Black-box job + kiểm tra leakage |

## 9. Ngoài phạm vi

VPS / cloud thật, self-hosted runner, observability stack nặng, dashboard đẹp, approval matrix phức tạp, đổi khỏi OpenSpec, tách OpenSpec theo team, BMAD, Kiro IDE, Jev hosted (chỉ giữ làm mốc so sánh nếu cần), fine-tune Kev-27B.

## 10. Câu hỏi còn mở

Đã đóng:
- ~~VPS nào cho staging/prod?~~ → dùng kind local có sẵn.
- ~~Kênh nhận alert?~~ → kênh local (webhook/log giả) cho PoC.
- ~~GitHub Actions trả phí / giới hạn phút?~~ → repo public (miễn phí), và tạm chạy local.
- ~~DGX làm self-hosted runner?~~ → không; DGX chỉ chạy Kev.

Còn mở:
- `[UNRESOLVED: Chuẩn compliance nội bộ công ty có thêm yêu cầu ngoài PDPA/ASVS/PCI không?]` (chỉ ảnh hưởng `gates/*.yaml` phần security/privacy, không chặn W0-W3)
- ~~Ghim OWASP ASVS 4.0.3 hay 5.0?~~ → 4.0.3 (ID ổn định, trích chắc chắn; `pins.asvs` trong `gates/*.yaml`). Đổi sang 5.0 = version mới của gate.

## 11. Ghi chú vận hành Kev (cập nhật trong lúc làm)

- Server: Kev-27B@v1.0, fp16, GPU 0-2 (~17 GB mỗi card, card 32 GB), `127.0.0.1:8012` trên `osg-dgx1`, tmux `kev27-8012`. Script: `~/Work/kev-tools/kev27.sh`.
- Hiện chạy kernel PyTorch tham chiếu (thiếu `causal_conv1d`, `flash-linear-attention`), chậm hơn đáng kể. Kết quả W0-S1 ghi vào đây. **2026-10-11:** server không chạy, cả 8 GPU đang bị job khác chiếm ~23.7 GB/card → chưa đo được; mọi thứ chạy với engine mock, `ENGINE_DOWN` fallback đã test.
- fp16 khác đường bf16/fp32 mà model được calibrate (V100 không có bf16). Không đo riêng; ngưỡng calibrate lại trên eval set (W3-6).
- Validated context của Kev-27B là 65,536 token, nhưng vẫn cắt state theo scenario để câu hỏi tập trung và rẻ.
- Tắt server khi không dùng: `ssh osg-dgx1 '~/Work/kev-tools/kev27.sh stop'`.

## 12. Rủi ro

| Rủi ro | Cách chặn |
|---|---|
| Agent tự chấm bài | Gate trong workflow + reviewer khác agent viết + red-first proof |
| Test viết cho khớp code | Tách test-author / implementer, hook khóa file test, mutation test |
| Kev sai một cách tự tin | Advisory trước, chỉ bật veto sau khi W3-6 có số; câu veto luôn có LLM/human làm lưới cuối với change rủi ro cao |
| **Kev không thấy lỗi ngoài checklist** | Eval set có nhóm lỗi ngoài checklist để đo; giữ một lượt LLM mở ("còn gì khác?") ở feature gate nếu số đo cho thấy cần |
| Câu hỏi viết dở | Lấy từ chuẩn có `source`; bộ câu hỏi có version, review như code |
| Spec các team lệch nhau | `cross_change_check.py` + contract slice đi trước + human khóa spec trước khi build |
| DGX / Kev chết | `ENGINE_DOWN` → fallback LLM reviewer, không chặn vì hạ tầng |
| Chi phí agent tăng | Đo trong W3-6 và W5-1; reviewer chuyên chỉ chạy khi script định tuyến tới |
| Prompt injection từ nội dung repo/log | Nội dung đọc vào là dữ liệu; agent không có credential prod; audit log mọi hành động ghi |
| Repo public | Không self-hosted runner; không đưa secret thật vào repo/CI; PoC chỉ dữ liệu giả |

## 13. Workflow initiative → team spec → orchestrator review → build

OpenSpec 1.12 không có change cha/con hay `depends_on`. Tầng initiative đặt ở `plans/<id>/` (ngoài `openspec/`, để không lệch schema của OpenSpec); mỗi change ghi `initiative: <id>` trong `proposal.md` để truy ngược.

```
1 Initiative   plans/<id>/00-spec.md  (orchestrator + human; parallel-scrum)
               └─ slices.yaml: mỗi slice = 1 OpenSpec change, owner = repo giữ FEATURES.yaml,
                  depends_on (contract trước), interface cần thống nhất
2 Team spec    song song, mỗi slice 1 subagent team-spec-author (chỉ đọc repo của mình + proto,
               chỉ ghi openspec/changes/<slice>/)  → proposal + delta spec + tasks
3 Spec review  orchestrator:
               a) từng change: openspec validate --strict + spec_lint + Kev (spec.v1: ISO 29148/EARS)
               b) chéo (cross_change_check.py): RPC/event slice dùng có trong contract slice; mỗi scenario
                  thuộc đúng 1 FEATURES.yaml; không 2 slice sửa cùng requirement; DAG không vòng
               c) fail → feedback về team-spec-author, tối đa 3 vòng rồi escalate human
               d) human duyệt → khóa spec (Locked)
4 Build        theo DAG: contract (platform-core, tuần tự) → service → gateway → frontend;
               mỗi slice chạy feature-run (e2e-author → test-author → implementer → task-reviewer)
5 Converge     features-check + e2e toàn bộ + feature-reviewer → archive từng change
```

`slices.yaml` (đề xuất):

```yaml
initiative: seller-payout-v2
slices:
  - id: payout-contract          # = tên OpenSpec change
    owner: platform-core
    depends_on: []
  - id: payout-holdback
    owner: team-payment
    depends_on: [payout-contract]
    uses: [PaymentService.HoldPayout, payment.events/PayoutHeld]
  - id: payout-seller-ui
    owner: team-frontend
    depends_on: [payout-holdback]
```

Ràng buộc:
- Orchestrator là phiên chính (skill) hoặc Workflow script, vì subagent không gọi được subagent.
- Có điểm human duyệt ở bước 3d → tách 2 lượt chạy: `initiative-plan` (1-3) và `initiative-build` (4-5).
- Profile `solo`: team = subagent. Profile `team`: team = người thật + phiên Claude của họ, nộp change qua PR; orchestrator chạy bước 3 trên PR đó. Quy trình không đổi.

## 14. Thí nghiệm Kev (A/B/C)

**Câu hỏi:** Với cùng một checklist, Kev + escalate có bắt lỗi ngang LLM reviewer quét toàn bộ không, rẻ và nhanh hơn bao nhiêu, và bỏ sót loại lỗi nào?

**Ba nhánh trên cùng bộ input:**
- **A:** LLM reviewer quét toàn bộ diff + spec (cách hiện tại).
- **B:** Kev trả lời checklist theo từng scenario; câu không chắc mới escalate sang LLM, chỉ đúng câu đó.
- **C:** chỉ Kev (để biết escalate gánh bao nhiêu).
- Nhánh phụ: Kev-4B trên GPU 3-7 thay 27B ở B/C.

**Ground truth bằng lỗi cài (seeded defects):** lấy diff thật từ 51 change đã archive, cài lỗi có kiểm soát, mỗi lỗi gắn ID chuẩn: assertion chỉ check status code, test chỉ assert mock được gọi, thiếu case lỗi/quyền, thiếu case biên, code làm nhiều hơn test yêu cầu, scenario không có test. Thêm một nhóm **lỗi ngoài checklist** để đo giới hạn của B/C. Human chỉ gán nhãn một mẫu nhỏ để kiểm tra chéo.

**Metric mỗi nhánh:** recall/precision theo từng câu (và theo ID chuẩn), tỉ lệ escalate (B), token LLM, giờ GPU + latency Kev, wall-clock mỗi feature.

**Quy tắc:**
- Ngưỡng `threshold` / `min_confidence` đặt từ đường precision/recall, không đặt tay.
- Kev chỉ chuyển từ advisory sang chặn thật với những câu đạt recall đủ cao trên eval set.
- Eval set và kết quả có version; sửa câu hỏi → chạy lại trên state đã lưu.

**Done:** bảng A/B/C theo từng câu; kết luận câu nào giao Kev, câu nào giữ LLM; chi phí tiết kiệm được; 4B dùng được cho gate nào.
