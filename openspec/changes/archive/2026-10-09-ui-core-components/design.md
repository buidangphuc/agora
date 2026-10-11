## Context

Ant Design is the reference for the component set, prop naming and state behaviour; agora implements it
natively with Tailwind tokens (`ui-foundation`). Existing primitives were written in the WIP and are kept
API-compatible because ~47 files already import them.

## Goals / Non-Goals

Goals: a complete core set for the six phases, one state contract, a11y built in, testable in isolation.
Non-goals: full Ant parity, theming API, charts, `antd` dependency.

## Decisions

1. **Ant Design mapping**

   | Ant group | Ant component | agora component |
   |---|---|---|
   | General | Button, Typography | `Button` (variants primary/secondary/outline/ghost/danger/white ≈ Ant type/danger/ghost) |
   | Layout | Grid, Space, Divider | Tailwind grid/gap (no component) |
   | Navigation | Breadcrumb, Pagination, Steps, Tabs | `Breadcrumb`, `Pagination`, `Stepper`, `Tabs` |
   | Data Entry | Form.Item, Input, InputNumber, Select, Checkbox, Radio, Rate | `FormItem`, `Input`, `QuantityPicker`, `Select`, `Checkbox`, `Radio`, `Rate` |
   | Data Display | Avatar, Badge, Card, Descriptions, Empty, Image, Statistic, Table, Tag, Timeline, Skeleton | same names (`Badge` keeps count/dot role, `Tag` takes the label role) |
   | Feedback | Alert, Drawer, Modal, message, Progress, Result, Spin | `Alert`, `Drawer`, `Modal`, `ToastProvider`, `Progress`, `Result`, `Spin` |

2. **Server-first.** Components without interaction are server-compatible (no `"use client"`): Badge, Tag,
   Card, Descriptions, Statistic, Result, Empty, Skeleton, Timeline, Breadcrumb, Pagination (links), Image
   wrapper, Alert without close. Only Modal, Drawer, Tabs, QuantityPicker, Rate input, Select/Checkbox/Radio
   controlled variants, ToastProvider are client islands.
3. **Tier 3 tokens** are `const` class maps at the top of each file keyed by variant/size, referencing Tier 2
   aliases only (`bg-action-primary`, `text-text-secondary`, ...).
4. **Focus**: `focus-visible:ring-2 ring-focus-ring ring-offset-2`; never `focus:` alone.
5. **Dialog a11y** shared by Modal and Drawer via a small internal `useDialog` hook (focus trap, Escape,
   return focus, scroll lock). No new dependency.
6. **Badge vs Tag**: Ant splits count badges from label tags. Existing `Badge` variants used as labels
   (mall, discount, freeship) stay working; new code uses `Tag` for labels.
7. **Tests**: one Vitest + RTL file per component covering its scenarios; `/dev/ui` is rendered in one
   smoke test.

## Risks / Trade-offs

- Additive-only API for existing primitives limits clean-up; acceptable to avoid touching 47 call sites now.
- A hand-written focus trap may miss edge cases (iframes, shadow DOM) — not used in this app.

## Open Questions

None blocking.
