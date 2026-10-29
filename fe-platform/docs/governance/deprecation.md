# Deprecation policy

1. Deprecate in a minor release: add a JSDoc tag on the export, `@deprecated Use <replacement>; removed in <next major>.` The catalog (`docs/agents/catalog.json`) then carries a `deprecated` field and agents stop choosing it.
2. Keep it working and list it in the changelog under the same minor.
3. Remove it only in the next major, and only after at least one minor has shipped with the tag.
4. A token or class-map key follows the same path: add the new name, mark the old one in the changelog, remove in the major with a `## Migration` section.

Check: `npm run catalog:check` keeps the catalog in step with the tag; the reviewer confirms the removal version.
