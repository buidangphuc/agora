## Purpose

Defines the core UI component set of `team-frontend` (`src/components/ui/`), modelled on Ant Design's
component groups and states and implemented on the agora design tokens, so every screen composes the same
building blocks with the same behaviour.

## ADDED Requirements

### Requirement: Interactive components share one state contract

Every interactive component (Button, Input, Select, Checkbox, Radio, QuantityPicker, Rate input, Tabs,
Pagination, Tag when closable, Breadcrumb links) SHALL implement: default; hover (150–200ms transition);
active (`active:scale-95` for buttons, darker fill); focus-visible (2px ring using the `focus-ring` alias,
visible only for keyboard focus); disabled (native `disabled` where the element supports it, plus
`aria-disabled="true"`, no pointer events, 50% opacity); and, where an action can be pending, loading
(`isLoading` prop, spinner, width preserved, activation blocked, `aria-busy="true"`).

#### Scenario: A loading button keeps its width and blocks clicks

- **WHEN** a `Button` labelled "Thêm vào giỏ" is rendered with `isLoading` after being rendered without it
- **THEN** its rendered width is unchanged, it shows a spinner, `aria-busy` is `true`, and a click does not
  call `onClick`

#### Scenario: Disabled is announced and inert

- **WHEN** any interactive component is rendered with `disabled`
- **THEN** it carries `aria-disabled="true"`, is skipped or inert for clicks, and keyboard activation does
  nothing

#### Scenario: Focus ring shows for keyboard users only

- **WHEN** a user tabs to a `Button` on the `/dev/ui` catalogue
- **THEN** a visible 2px focus ring is drawn; when the same button is clicked with a mouse, no ring is drawn

### Requirement: Data components define empty, error and loading states

Data-display components that take collections or async content (Table, Timeline, Descriptions, Statistic,
Card with `loading`, Image) SHALL render: a `Skeleton` of the same footprint while loading, an `Empty`
(illustration, description, optional action) when the collection is empty, and an inline `Alert` with a
retry action when given an error.

#### Scenario: An empty table shows Empty, not a blank area

- **WHEN** a `Table` is rendered with `dataSource=[]`
- **THEN** it renders the column headers and an `Empty` block with the configured description

#### Scenario: A loading statistic does not shift layout

- **WHEN** a `Statistic` switches from `loading` to a value
- **THEN** the element's bounding box height is unchanged

### Requirement: The existing primitives meet the contract

`Button`, `Input`, `Badge`, `Card`, `Modal`, `PriceTag`, `Result`, `Descriptions`, `Statistic`, `Stepper`
and `Tabs` SHALL keep their current public props (additive changes only) and SHALL meet the shared state
contract, use only Tier 2/3 tokens, and meet these a11y rules: `Modal` has `role="dialog"`,
`aria-modal="true"`, `aria-labelledby` its title, traps focus, closes on Escape and returns focus to the
trigger; `Tabs` uses `role="tablist"`/`tab`/`tabpanel` with arrow-key navigation; `Input` links its label
and error via `htmlFor` and `aria-describedby` and sets `aria-invalid` on error; `Stepper` marks the current
step with `aria-current="step"`. `Tabs` SHALL also offer a link variant (`hrefFor(id)`, server-compatible) for
tab state held in URL searchParams, rendered as a navigation list with `aria-current="page"`.

#### Scenario: Modal traps focus and restores it

- **WHEN** a user opens a `Modal` from a button, presses Tab repeatedly, then presses Escape
- **THEN** focus cycles inside the dialog only, the dialog closes on Escape, and focus returns to the
  opening button

#### Scenario: Tabs are keyboard navigable

- **WHEN** focus is on the first tab and the user presses ArrowRight
- **THEN** the second tab becomes selected and its panel is shown

#### Scenario: Link tabs keep their state in the URL without client JavaScript

- **WHEN** `Tabs` is rendered in a server component with `hrefFor=(id)=>"/account/orders?status="+id` and
  `activeId="shipping"`
- **THEN** each tab is a link to its `hrefFor` URL, the `shipping` tab carries `aria-current="page"`, and
  the browser back button restores the previous tab

#### Scenario: Input errors are announced

- **WHEN** an `Input` with label "Email" is rendered with `error="Email không hợp lệ"`
- **THEN** the input has `aria-invalid="true"` and its accessible description contains the error text

### Requirement: Missing core components are provided

The component set SHALL include, each exported from `src/components/ui/index.ts`: `Breadcrumb`,
`Pagination` (URL-driven via `hrefFor(page)` so it works in server components), `FormItem`, `Select`
(native select styling), `Checkbox`, `Radio`/`RadioGroup`, `QuantityPicker` (min, max, step; buttons disable
at bounds), `Rate`, `Tag` (colour presets from aliases, optional close), `Table`, `Timeline`, `Skeleton`
(text, avatar, image, card presets), `Empty`, `Avatar`, `Image` (required `aspect` prop, fallback on error,
`loading="lazy"` by default), `Alert` (info/success/warning/error, optional action), `Spin`, `Drawer`
(same dialog a11y as Modal) and `Progress` (line, with `aria-valuenow`).

#### Scenario: QuantityPicker respects its bounds

- **WHEN** a `QuantityPicker` with `min=1 max=3` shows 3
- **THEN** the increment button is disabled, typing 5 clamps to 3, and the decrement button stays enabled

#### Scenario: Pagination works without client JavaScript

- **WHEN** `Pagination` is rendered in a server component with `current=2 total=50 pageSize=10` and
  `hrefFor=(p)=>"/search?page="+p`
- **THEN** it renders links to pages 1–5 with page 2 marked `aria-current="page"`

#### Scenario: Image reserves its box and falls back

- **WHEN** an `Image` with `aspect="square"` is rendered with a URL that fails to load
- **THEN** its box is square before and after the failure and the fallback placeholder is shown

### Requirement: A dev catalogue renders every component in every state

`/dev/ui` SHALL render every exported component in each of its states, SHALL be available only when
`NODE_ENV !== "production"`, and SHALL be the fixture used for visual review.

#### Scenario: The catalogue is not shipped to production

- **WHEN** the production build is requested at `/dev/ui`
- **THEN** it returns 404
