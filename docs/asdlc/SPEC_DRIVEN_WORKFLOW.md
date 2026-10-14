# Plan: Spec-Driven Workflow (OpenSpec → e2e + code, song song)

Mục tiêu: mỗi requirement mới đi qua **một pipeline chuẩn** —
`requirement → OpenSpec change → (e2e ∥ code) → validate → archive` — để nhiều
agent/session làm song song mà không đụng nhau, và **e2e có mặt từ lúc spec** chứ
không phải làm sau.

## Quyết định đã chốt
- **Cài OpenSpec CLI thật** ở root polyrepo (không tự chế).
- **Thay thế speckit**: gỡ `team-ai/.claude/skills/speckit-*` + `team-ai/.specify/`; team-ai trở thành "một repo trong `services:`" của luồng chung.
- Bối cảnh: `platform-e2e` đang **34/34 automated (100%)** + đã có `FEATURES.yaml` (spec máy đọc) + loader/gate `make features-check`. Luồng mới **tái dùng** hạ tầng này, OpenSpec là lớp requirement ở trên.

## Vì sao OpenSpec giúp parallel (điểm mấu chốt)
1. **Change cô lập**: mỗi requirement = 1 thư mục `openspec/changes/<id>/` (proposal + tasks + spec-delta). N change = N thư mục rời → N agent author/implement đồng thời, gần như zero merge-conflict.
2. **Trong 1 change, tách 2 track chạy song song**: `code` (theo repo) và `e2e` (platform-e2e). E2e viết theo spec-delta được ngay, chạy đỏ→xanh khi code về — không phải chờ code xong mới viết test.
3. **Scenario của OpenSpec ≈ Gherkin**: spec-delta dùng block `#### Scenario:` (WHEN/THEN) → ánh xạ thẳng sang `FEATURES.yaml.acceptance` và `.feature`. Một nguồn, hai đầu ra (code + e2e).
4. Hội tụ bằng **gate tự động** (validate + features-check + pytest xanh), rồi `openspec archive` gộp delta vào `specs/`.

## Pipeline
```
requirement (NL)
   │  skill: spec-propose
   ▼
openspec/changes/<id>/            ← 1 change, cô lập
   ├─ proposal.md   (why/what)
   ├─ tasks.md      (checklist code + e2e)
   ├─ design.md     (tùy chọn)
   ├─ specs/<capability>/spec.md  (## ADDED/MODIFIED Requirements + Scenario)
   └─ features.delta.yaml          ← e2e contract (mảnh FEATURES.yaml)  ★shift-left
   │
   ├───────────────┬───────────────────────────  (FAN-OUT song song)
   ▼               ▼
skill: spec-to-code      skill: spec-to-e2e
 (per repo Go/TS/Py)      (platform-e2e: .feature+steps+pages)
 parallel-scrum +         manifest→test loop đã có
 worktree/agent            → chạy stack → flip status: automated
   │               │
   └──────┬────────┘
          ▼  gate hội tụ
  openspec validate --strict  &&  make features-check  &&  pytest xanh  &&  lint
          ▼
  skill: spec-archive → gộp delta vào openspec/specs/ ; FEATURES.yaml đã cập nhật
```

## Quy ước ánh xạ (glue để cả người lẫn agent theo được)
| OpenSpec | platform-e2e |
|---|---|
| `changes/<id>/specs/<capability>/spec.md` (Requirement + Scenario) | `<repo>/FEATURES.yaml` feature (`acceptance` = Scenario) |
| capability (vd `engagement`) | repo sở hữu + `feature.id` (`engagement.wishlist`) |
| Scenario block | `.feature` Scenario + `covered_by` |
| `openspec archive` | feature `status: automated` đã set khi e2e xanh |
Nguyên tắc cũ giữ nguyên: feature thuộc repo **sở hữu capability**; `services:` liệt kê mọi repo liên quan.

## Skills — tạo mới (đặt ở **root** `.claude/skills/`, áp dụng toàn polyrepo)
1. **`spec-propose`** — requirement NL → gọi luồng "create change" của OpenSpec, **và** sinh `features.delta.yaml` (mảnh FEATURES.yaml cho capability bị đụng) trong change. Kết quả: spec + e2e-contract + tasks, sẵn sàng fan-out.
2. **`spec-to-e2e`** — từ change: đọc Scenario + `features.delta.yaml` → merge vào `<repo>/FEATURES.yaml`, scaffold `.feature`/steps/pages theo vòng lặp chuẩn của platform-e2e, chạy stack, flip `status: automated` + `covered_by`. (Tái dùng `make features-check`.)
3. **`spec-to-code`** — từ `tasks.md` → hiện thực theo từng repo (tuân AGENTS.md: gateway-only, proto là nguồn, mỗi service 1 DB…). Dispatch song song bằng `parallel-scrum` + worktree.
4. **`spec-dispatch`** (orchestrator) — nhận change đã duyệt → fan-out `spec-to-code` (nhiều repo) ∥ `spec-to-e2e` như các subagent/worktree, rồi chạy gate hội tụ. Đây là chỗ "parallel nhiều" mà bạn muốn.

## Skills — gỡ / thay
- Xoá `team-ai/.claude/skills/speckit-*` và `team-ai/.specify/`.
- Nếu có tài liệu/tham chiếu speckit trong team-ai → trỏ sang `openspec/` root.
- Cập nhật root `AGENTS.md` + `CLAUDE.md`: thêm mục "Spec-driven flow (OpenSpec)"; OpenSpec init cũng tự chèn hướng dẫn agent — rà lại để không trùng lặp.

## Glue tooling cần build (nhẹ)
- `platform-e2e/scripts/spec_sync.py`: input = change id → đọc Scenario trong spec-delta, đối chiếu `FEATURES.yaml`, báo "scenario nào của change đã có e2e xanh" → điều kiện để được `archive`. Bổ sung target `make spec-check CHANGE=<id>`.
- `openspec/project.md`: chép rule kiến trúc từ `platform-core/docs` + root `AGENTS.md` (bounded context, gateway edge, CQRS, proto) để OpenSpec propose đúng ranh giới.

## Rollout theo phase (mỗi phase là 1 mốc verify được)
- **P0 — Cài đặt**: `openspec init` ở root (xác nhận lệnh/flag theo docs OpenSpec lúc cài); viết `openspec/project.md`; gỡ speckit. → verify: `openspec list` chạy, `team-ai/.specify` đã xoá.
- **P1 — Skills**: tạo 4 skill trên; wire vào root `.claude/`. → verify: gọi `spec-propose` sinh ra 1 change hợp lệ (`openspec validate <id> --strict` pass).
- **P2 — Glue**: `spec_sync.py` + `make spec-check`. → verify: chạy trên 1 change mẫu, báo coverage đúng.
- **P3 — Pilot end-to-end**: chọn 1 requirement thật nhỏ (vd `engagement.wishlist` hoặc feature còn thiếu), chạy trọn pipeline: propose → dispatch (code∥e2e) → gate → archive. → verify: feature mới `automated`, spec vào `openspec/specs/`, PR sạch.
- **P4 — Migrate & doc**: chuyển nhu cầu SDD của team-ai sang luồng chung; cập nhật AGENTS/CLAUDE; ghi "cách thêm requirement mới" 1 trang.

## Cần xác nhận khi thực thi (không đoán trước)
- Lệnh/flag OpenSpec chính xác (`init`, `validate --strict`, `archive`, layout `changes/`) theo **docs bản OpenSpec cài về** — plan này bám cấu trúc proposal/tasks/spec-delta/scenario nhưng chốt lại lúc cài.
- OpenSpec cài qua npm (`npx openspec` hay global) — cần network; xác nhận môi trường CI cho phép.
- Có giữ `design.md` bắt buộc cho change lớn không (khuyến nghị: chỉ khi có quyết định kỹ thuật cross-service).

## Ngoài phạm vi
- Không đụng bds-qa-e2e-web `/qa` skills (repo khác).
- Chưa tự động sinh code service từ spec (P3 vẫn người/agent viết theo tasks); chỉ tự động phần e2e scaffolding vốn đã có loop.
