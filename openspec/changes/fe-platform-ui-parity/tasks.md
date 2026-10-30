## 1. Tokens

- [x] 1.1 Add primitive `danger-darker`, semantic `action-danger` and `action-danger-hover` for light and dark, their text-inverse pairs in `pairs.mjs`, and class-map entries for button `danger`, `xs`, `fullWidth`, `icon`, `spinner` and the card parts; verify `npm run test -w packages/tokens` and the contrast gate are green

## 2. Components

- [x] 2.1 Extend `Button` (`danger`, `xs`, `fullWidth`, `leftIcon`, `rightIcon`, spinner while `loading`); verify unit tests for each prop and the busy and disabled state
- [x] 2.2 Extend `Card` (`padded`, `hoverable`, `loading`) and add `CardHeader`, `CardTitle`, `CardDescription`, `CardContent`, `CardFooter`; export them; verify unit tests and that `className` and `style` are still rejected in `props.typecheck.tsx`
- [x] 2.3 Add the new states to `states.tsx`; verify axe finds no violation on any of them

## 3. Release records

- [x] 3.1 Add changesets (tokens minor, ui minor) and run `npm run catalog`; also run `npm run vendor`; verify `catalog:check`, `vendor:check` and `release:status` are green

## 4. Done-condition

- [x] 4.1 `npm run check` exits 0 in `fe-platform`; `openspec validate fe-platform-ui-parity --strict` passes
